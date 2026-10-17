// Headless engine-sim renderer: runs an engine script offline under a
// scripted RPM and throttle plan and writes the synthesiser output as WAV.

#include "scripting/include/compiler.h"

#include "include/engine.h"
#include "include/simulator.h"
#include "include/units.h"

#include <algorithm>
#include <cmath>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <vector>

#include <unistd.h>

#ifndef ENGINE_SIM_DATA_DIR
#define ENGINE_SIM_DATA_DIR ""
#endif

namespace fs = std::filesystem;

namespace {

constexpr int SynthRate = 44100;

enum class Mode { Hold, Sweep, Free };

struct Segment {
    Mode mode;
    double rpm0 = 0;
    double rpm1 = 0;
    double throttle = 0;
    double seconds = 0;
};

struct Options {
    std::string script;
    std::string out;
    std::string csv;
    std::string dataDir = ENGINE_SIM_DATA_DIR;
    std::vector<Segment> segments;
    double settle = 3.0;
    double volume = 1.0;
    int rate = 48000;
    int csvEvery = 1;
};

[[noreturn]] void die(const std::string &msg) {
    std::cerr << "engine-sim-render: " << msg << "\n";
    std::exit(1);
}

void usage() {
    std::cerr <<
        "usage: engine-sim-render --script ENGINE.mr --out OUT.wav [segments] [options]\n"
        "\n"
        "Segments run in order; at least one is required:\n"
        "  --hold RPM:THROTTLE:SECONDS          dyno holds RPM\n"
        "  --sweep RPM0:RPM1:THROTTLE:SECONDS   dyno target ramps linearly\n"
        "  --free THROTTLE:SECONDS              dyno off, engine runs free\n"
        "\n"
        "Options:\n"
        "  --settle SECONDS   unrecorded pre-roll at the first segment state (default 3)\n"
        "  --volume GAIN      synthesiser output gain after the leveller (default 1, as the app;\n"
        "                     the app clips a few per cent of samples, 0.5 avoids that)\n"
        "  --rate HZ          output sample rate (default 48000, synthesiser runs at 44100)\n"
        "  --csv PATH         per-step dump: time, RPM, throttle, dyno torque, per-cylinder\n"
        "                     exhaust runner pressure and exhaust flow\n"
        "  --csv-every N      write every Nth simulation step to the CSV (default 1)\n"
        "  --data-dir DIR     directory that holds engine-sim es/ (default: build prefix)\n"
        "\n"
        "THROTTLE is engine-sim speed control in 0..1, the value of the app's Q/W/E/R keys.\n";
}

std::vector<double> parseNumbers(const std::string &s, size_t count, const std::string &flag) {
    std::vector<double> out;
    std::stringstream ss(s);
    std::string part;
    while (std::getline(ss, part, ':')) {
        char *end = nullptr;
        const double v = std::strtod(part.c_str(), &end);
        if (part.empty() || *end != '\0' || !std::isfinite(v)) die("bad number in " + flag + " " + s);
        out.push_back(v);
    }
    if (out.size() != count) die(flag + " expects " + std::to_string(count) + " colon-separated values");
    return out;
}

Options parseArgs(int argc, char **argv) {
    Options o;
    for (int i = 1; i < argc; ++i) {
        const std::string a = argv[i];
        if (a == "-h" || a == "--help") { usage(); std::exit(0); }
        if (i + 1 >= argc) die("missing value for " + a);
        const std::string v = argv[++i];
        if (a == "--script") o.script = v;
        else if (a == "--out") o.out = v;
        else if (a == "--csv") o.csv = v;
        else if (a == "--data-dir") o.dataDir = v;
        else if (a == "--settle") o.settle = parseNumbers(v, 1, a)[0];
        else if (a == "--volume") o.volume = parseNumbers(v, 1, a)[0];
        else if (a == "--rate") o.rate = static_cast<int>(parseNumbers(v, 1, a)[0]);
        else if (a == "--csv-every") o.csvEvery = std::max(1, static_cast<int>(parseNumbers(v, 1, a)[0]));
        else if (a == "--hold") {
            const auto n = parseNumbers(v, 3, a);
            o.segments.push_back({ Mode::Hold, n[0], n[0], n[1], n[2] });
        }
        else if (a == "--sweep") {
            const auto n = parseNumbers(v, 4, a);
            o.segments.push_back({ Mode::Sweep, n[0], n[1], n[2], n[3] });
        }
        else if (a == "--free") {
            const auto n = parseNumbers(v, 2, a);
            o.segments.push_back({ Mode::Free, 0, 0, n[0], n[1] });
        }
        else { usage(); die("unknown option " + a); }
    }
    if (o.script.empty() || o.out.empty() || o.segments.empty()) { usage(); std::exit(2); }
    for (const Segment &s : o.segments) {
        if (s.seconds <= 0) die("segment duration must be positive");
        if (s.throttle < 0 || s.throttle > 1) die("throttle must be in 0..1");
        if (s.mode != Mode::Free && (s.rpm0 <= 0 || s.rpm1 <= 0)) die("RPM must be positive");
    }
    if (o.settle < 0) die("--settle must not be negative");
    if (o.volume <= 0 || o.volume > 1) die("--volume must be in (0, 1]");
    if (o.rate < 8000 || o.rate > 192000) die("--rate must be in 8000..192000");
    return o;
}

// Reads the first channel of a 16-bit PCM WAV file.
std::vector<int16_t> readWav16(const std::string &path) {
    std::ifstream f(path, std::ios::binary);
    if (!f) die("cannot open impulse response " + path);
    std::vector<char> b((std::istreambuf_iterator<char>(f)), std::istreambuf_iterator<char>());
    auto u16 = [&](size_t p) { return static_cast<uint16_t>((uint8_t)b[p] | ((uint8_t)b[p + 1] << 8)); };
    auto u32 = [&](size_t p) { return static_cast<uint32_t>(u16(p) | (uint32_t(u16(p + 2)) << 16)); };
    if (b.size() < 12 || std::string(b.data(), 4) != "RIFF" || std::string(b.data() + 8, 4) != "WAVE") {
        die("not a RIFF WAVE file: " + path);
    }
    int channels = 0, bits = 0, format = 0;
    for (size_t p = 12; p + 8 <= b.size();) {
        const std::string id(b.data() + p, 4);
        const uint32_t len = u32(p + 4);
        const size_t body = p + 8;
        if (body + len > b.size()) die("truncated WAV chunk in " + path);
        if (id == "fmt " && len >= 16) {
            format = u16(body);
            channels = u16(body + 2);
            bits = u16(body + 14);
        }
        else if (id == "data") {
            if (format != 1 || bits != 16 || channels < 1) die("impulse response is not 16-bit PCM: " + path);
            const size_t frames = len / (2 * channels);
            std::vector<int16_t> out(frames);
            for (size_t i = 0; i < frames; ++i) out[i] = static_cast<int16_t>(u16(body + i * 2 * channels));
            return out;
        }
        p = body + len + (len & 1);
    }
    die("no data chunk in " + path);
}

void writeWav16(const std::string &path, const std::vector<float> &x, int rate) {
    std::ofstream f(path, std::ios::binary);
    if (!f) die("cannot write " + path);
    auto u16 = [&](uint16_t v) { f.put(char(v & 0xff)); f.put(char(v >> 8)); };
    auto u32 = [&](uint32_t v) { u16(uint16_t(v & 0xffff)); u16(uint16_t(v >> 16)); };
    const uint32_t dataBytes = static_cast<uint32_t>(x.size() * 2);
    f.write("RIFF", 4); u32(36 + dataBytes); f.write("WAVE", 4);
    f.write("fmt ", 4); u32(16); u16(1); u16(1); u32(rate); u32(rate * 2); u16(2); u16(16);
    f.write("data", 4); u32(dataBytes);
    for (float s : x) u16(static_cast<uint16_t>(static_cast<int16_t>(std::lround(std::clamp(s, -32768.0f, 32767.0f)))));
    if (!f) die("write failed: " + path);
}

// Windowed-sinc (Blackman) resampler for the offline 44.1 kHz to output rate step.
std::vector<float> resample(const std::vector<int16_t> &in, int from, int to) {
    if (from == to) return std::vector<float>(in.begin(), in.end());
    constexpr int HalfTaps = 32;
    const double ratio = double(to) / from;
    const double cutoff = 0.97 * std::min(1.0, ratio);
    const size_t n = static_cast<size_t>(std::floor(in.size() * ratio));
    std::vector<float> out(n);
    for (size_t i = 0; i < n; ++i) {
        const double t = i / ratio;
        const long c = static_cast<long>(std::floor(t));
        double acc = 0;
        for (long k = c - HalfTaps + 1; k <= c + HalfTaps; ++k) {
            if (k < 0 || k >= static_cast<long>(in.size())) continue;
            const double d = t - k;
            const double x = M_PI * d * cutoff;
            const double sinc = (std::abs(x) < 1e-9) ? 1.0 : std::sin(x) / x;
            const double w = 0.42 + 0.5 * std::cos(M_PI * d / HalfTaps) + 0.08 * std::cos(2 * M_PI * d / HalfTaps);
            acc += in[k] * cutoff * sinc * w;
        }
        out[i] = static_cast<float>(acc);
    }
    return out;
}

struct Control {
    bool dyno;
    double rpm;
    double throttle;
};

Control controlAt(const std::vector<Segment> &segs, double t) {
    for (const Segment &s : segs) {
        if (t < s.seconds) {
            const double f = t / s.seconds;
            return { s.mode != Mode::Free, s.rpm0 + (s.rpm1 - s.rpm0) * f, s.throttle };
        }
        t -= s.seconds;
    }
    const Segment &s = segs.back();
    return { s.mode != Mode::Free, s.rpm1, s.throttle };
}

} // namespace

int main(int argc, char **argv) {
    const Options opt = parseArgs(argc, argv);

    const fs::path script = fs::absolute(opt.script);
    const fs::path out = fs::absolute(opt.out);
    const fs::path csvPath = opt.csv.empty() ? fs::path() : fs::absolute(opt.csv);
    if (!fs::exists(script)) die("script not found: " + script.string());
    if (opt.dataDir.empty() || !fs::exists(fs::path(opt.dataDir) / "es" / "engine_sim.mr")) {
        die("engine-sim es/ library not found; pass --data-dir");
    }

    // The script compiler resolves es/ relative to the working directory
    // and writes error_log.log there, so work in a scratch directory.
    const fs::path work = fs::temp_directory_path() / ("engine-sim-render-" + std::to_string(::getpid()));
    fs::create_directories(work);
    fs::create_directory_symlink(fs::absolute(opt.dataDir) / "es", work / "es");
    {
        std::ofstream mainMr(work / "main.mr");
        mainMr << "import \"engine_sim.mr\"\nimport \"" << script.string() << "\"\n\nmain()\n";
    }
    fs::current_path(work);

    es_script::Compiler compiler;
    compiler.initialize();
    if (!compiler.compile((work / "main.mr").string().c_str())) {
        std::ifstream log(work / "error_log.log");
        std::cerr << log.rdbuf();
        fs::remove_all(work);
        die("script compilation failed");
    }
    const es_script::Compiler::Output output = compiler.execute();
    compiler.destroy();

    Engine *engine = output.engine;
    Vehicle *vehicle = output.vehicle;
    Transmission *transmission = output.transmission;
    if (engine == nullptr || vehicle == nullptr || transmission == nullptr) {
        die("script did not set an engine, vehicle and transmission (does it define main?)");
    }

    Simulator *sim = engine->createSimulator(vehicle, transmission);
    sim->setSimulationFrequency(static_cast<int>(engine->getSimulationFrequency()));
    // Offline: drain the synthesiser every frame, so no latency target.
    sim->setTargetSynthesizerLatency(0.0);

    Synthesizer &synth = sim->synthesizer();
    Synthesizer::AudioParameters audio = synth.getAudioParameters();
    audio.inputSampleNoise = static_cast<float>(engine->getInitialJitter());
    audio.airNoise = static_cast<float>(engine->getInitialNoise());
    audio.dF_F_mix = static_cast<float>(engine->getInitialHighFrequencyGain());
    audio.volume = static_cast<float>(opt.volume);
    synth.setAudioParameters(audio);

    for (int i = 0; i < engine->getExhaustSystemCount(); ++i) {
        ImpulseResponse *ir = engine->getExhaustSystem(i)->getImpulseResponse();
        fs::path irPath = ir->getFilename();
        if (irPath.is_relative()) irPath = work / irPath;
        const std::vector<int16_t> samples = readWav16(irPath.string());
        synth.initializeImpulseResponse(samples.data(), static_cast<unsigned int>(samples.size()),
            static_cast<float>(ir->getVolume()), i);
    }

    fs::remove_all(work);

    std::vector<int16_t> discard(synth.m_audioBuffer.size());
    synth.readAudioOutput(static_cast<int>(discard.size()), discard.data());

    std::ofstream csv;
    const int cylinders = engine->getCylinderCount();
    if (!csvPath.empty()) {
        csv.open(csvPath);
        if (!csv) die("cannot write " + csvPath.string());
        csv << "time_s,rpm,throttle,dyno_torque_nm";
        for (int i = 0; i < cylinders; ++i) csv << ",runner_pressure_pa_" << i;
        for (int i = 0; i < cylinders; ++i) csv << ",exhaust_flow_mol_" << i;
        csv << "\n";
        csv.precision(9);
    }

    double total = 0;
    for (const Segment &s : opt.segments) total += s.seconds;
    const double endTime = opt.settle + total;
    const double frame = 1.0 / 200.0;
    const double timestep = sim->getTimestep();

    engine->getIgnitionModule()->m_enabled = true;
    sim->m_starterMotor.m_enabled = true;
    sim->m_dyno.m_hold = true;

    std::vector<int16_t> rendered;
    rendered.reserve(static_cast<size_t>(endTime * SynthRate) + SynthRate);
    std::vector<int16_t> chunk(4096);
    size_t settleSamples = 0;
    long long step = 0;
    double t = 0;

    while (t < endTime) {
        const double segT = t - opt.settle;
        const Control c = controlAt(opt.segments, std::max(0.0, segT));
        // The starter cranks for the first half second of the pre-roll.
        sim->m_starterMotor.m_enabled = t < 0.5;
        sim->m_dyno.m_enabled = c.dyno && t >= 0.5;
        sim->m_dyno.m_rotationSpeed = units::rpm(c.rpm);
        engine->setSpeedControl(c.throttle);

        sim->startFrame(frame);
        while (sim->simulateStep()) {
            ++step;
            if (csv.is_open() && step % opt.csvEvery == 0) {
                csv << step * timestep - opt.settle << ',' << engine->getRpm() << ',' << c.throttle
                    << ',' << sim->m_dyno.getTorque();
                for (int i = 0; i < cylinders; ++i) csv << ',' << engine->getChamber(i)->m_exhaustRunnerAndPrimary.pressure();
                for (int i = 0; i < cylinders; ++i) csv << ',' << engine->getChamber(i)->getLastTimestepExhaustFlow();
                csv << '\n';
            }
        }
        sim->endFrame();

        while (synth.m_inputChannels[0].data.size() > 0) {
            synth.renderAudio();
            const int n = static_cast<int>(synth.m_audioBuffer.size());
            if (n > static_cast<int>(chunk.size())) chunk.resize(n);
            synth.readAudioOutput(n, chunk.data());
            rendered.insert(rendered.end(), chunk.begin(), chunk.begin() + n);
            synth.endInputBlock();
        }

        t = step * timestep;
        if (t <= opt.settle) settleSamples = rendered.size();
    }

    const size_t want = static_cast<size_t>(std::llround(total * SynthRate));
    const size_t start = std::min(settleSamples, rendered.size());
    std::vector<int16_t> kept(rendered.begin() + start, rendered.begin() + std::min(rendered.size(), start + want));
    writeWav16(out.string(), resample(kept, SynthRate, opt.rate), opt.rate);

    std::cerr << "engine-sim-render: " << engine->getName() << ", " << cylinders << " cylinders, "
              << engine->getExhaustSystemCount() << " exhaust systems, sim " << sim->getSimulationFrequency()
              << " Hz, final " << std::lround(engine->getRpm()) << " rpm, wrote "
              << double(kept.size()) / SynthRate << " s to " << out.string() << "\n";

    sim->releaseSimulation();
    delete sim;
    return 0;
}
