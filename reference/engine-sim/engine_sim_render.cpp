// Headless engine-sim renderer: runs an engine script offline under a
// scripted RPM and throttle plan and writes the synthesiser output as WAV.

#include "scripting/include/compiler.h"

#include "include/connecting_rod.h"
#include "include/crankshaft.h"
#include "include/cylinder_bank.h"
#include "include/cylinder_head.h"
#include "include/engine.h"
#include "include/exhaust_system.h"
#include "include/ignition_module.h"
#include "include/impulse_response.h"
#include "include/intake.h"
#include "include/piston.h"
#include "include/piston_engine_simulator.h"
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

// Counts every rand() call of the process, for the --dump-synth record of
// the calls outside the synthesiser. glibc defines rand() as (int) random(),
// so this definition returns the same values as the glibc one.
static long long g_randCalls = 0;
extern "C" int rand(void) noexcept {
    ++g_randCalls;
    return static_cast<int>(random());
}

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
    std::string dumpJson;
    std::string dumpSynth;
    std::string dataDir = ENGINE_SIM_DATA_DIR;
    std::vector<Segment> segments;
    bool noNoise = false;
    double settle = 3.0;
    double idleSeconds = 8.0;
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
        "       engine-sim-render --script ENGINE.mr --dump-json OUT.json [--idle-seconds SECONDS]\n"
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
        "  --dump-json PATH   write the engine that the script builds as JSON, with the idle rpm\n"
        "                     of a free run at speed control 0, instead of a WAV\n"
        "  --idle-seconds S   length of that free run (default 8); the last 2 s give the idle rpm\n"
        "  --dump-synth DIR   also write the synthesiser input of each exhaust channel, its\n"
        "                     parameters and its whole 44.1 kHz output to DIR\n"
        "  --no-noise         set the synthesiser jitter and air noise to 0, for a deterministic output\n"
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
        if (a == "--no-noise") { o.noNoise = true; continue; }
        if (i + 1 >= argc) die("missing value for " + a);
        const std::string v = argv[++i];
        if (a == "--script") o.script = v;
        else if (a == "--out") o.out = v;
        else if (a == "--csv") o.csv = v;
        else if (a == "--dump-json") o.dumpJson = v;
        else if (a == "--dump-synth") o.dumpSynth = v;
        else if (a == "--idle-seconds") o.idleSeconds = parseNumbers(v, 1, a)[0];
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
    if (!o.dumpJson.empty()) {
        if (!o.out.empty() || !o.segments.empty() || !o.csv.empty() || !o.dumpSynth.empty()) {
            die("--dump-json takes no --out, --csv, --dump-synth or segments");
        }
        if (o.idleSeconds < 3) die("--idle-seconds must be at least 3");
        // The idle run: no pre-roll, the starter cranks for 0.5 s, then the engine runs free at speed control 0.
        o.out = o.dumpJson;
        o.settle = 0;
        o.segments.push_back({ Mode::Free, 0, 0, 0, o.idleSeconds });
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

std::string jsonString(const std::string &s) {
    std::string o = "\"";
    for (const char c : s) {
        if (c == '"' || c == '\\') o += '\\';
        if (static_cast<unsigned char>(c) >= 0x20) o += c;
    }
    return o + "\"";
}

double deg(double rad) { return rad * 180.0 / M_PI; }

// Writes the engine that the script built: geometry, firing, exhaust and
// limiter values in SI units, angles in degrees and speeds in rpm.
void writeDump(const fs::path &path, Engine *engine, double idleRpm, double idleSeconds) {
    std::ofstream f(path);
    if (!f) die("cannot write " + path.string());
    f.precision(10);
    IgnitionModule *ign = engine->getIgnitionModule();
    f << "{\n  \"name\": " << jsonString(engine->getName())
      << ",\n  \"simulation_frequency_hz\": " << engine->getSimulationFrequency()
      << ",\n  \"redline_rpm\": " << units::toRpm(engine->getRedline())
      << ",\n  \"rev_limit_rpm\": " << units::toRpm(ign->getRevLimit())
      << ",\n  \"limiter_duration_s\": " << ign->getLimiterDuration()
      << ",\n  \"starter_speed_rpm\": " << units::toRpm(engine->getStarterSpeed())
      << ",\n  \"dyno_min_rpm\": " << units::toRpm(engine->getDynoMinSpeed())
      << ",\n  \"dyno_max_rpm\": " << units::toRpm(engine->getDynoMaxSpeed())
      << ",\n  \"hf_gain\": " << engine->getInitialHighFrequencyGain()
      << ",\n  \"noise\": " << engine->getInitialNoise()
      << ",\n  \"jitter\": " << engine->getInitialJitter()
      << ",\n  \"idle\": { \"measured_rpm\": " << idleRpm << ", \"free_run_s\": " << idleSeconds << " }";

    f << ",\n  \"crankshafts\": [";
    for (int i = 0; i < engine->getCrankshaftCount(); ++i) {
        Crankshaft *c = engine->getCrankshaft(i);
        f << (i ? "," : "") << "\n    { \"throw_m\": " << c->getThrow() << ", \"tdc_deg\": " << deg(c->getTdc())
          << ", \"journals_deg\": [";
        for (int j = 0; j < c->getRodJournalCount(); ++j) f << (j ? ", " : "") << deg(c->getRodJournalAngle(j));
        f << "] }";
    }
    f << "\n  ],\n  \"banks\": [";
    for (int i = 0; i < engine->getCylinderBankCount(); ++i) {
        CylinderBank *b = engine->getCylinderBank(i);
        f << (i ? "," : "") << "\n    { \"angle_deg\": " << deg(b->getAngle()) << ", \"bore_m\": " << b->getBore()
          << ", \"deck_height_m\": " << b->getDeckHeight() << ", \"cylinders\": " << b->getCylinderCount() << " }";
    }
    f << "\n  ],\n  \"intakes\": [";
    for (int i = 0; i < engine->getIntakeCount(); ++i) {
        Intake *in = engine->getIntake(i);
        f << (i ? "," : "") << "\n    { \"runner_length_m\": " << in->getRunnerLength()
          << ", \"plenum_area_m2\": " << in->getPlenumCrossSectionArea()
          << ", \"idle_throttle_plate_position\": " << in->getIdleThrottlePlatePosition() << " }";
    }
    f << "\n  ],\n  \"exhaust_systems\": [";
    for (int i = 0; i < engine->getExhaustSystemCount(); ++i) {
        ExhaustSystem *e = engine->getExhaustSystem(i);
        ImpulseResponse *ir = e->getImpulseResponse();
        f << (i ? "," : "") << "\n    { \"length_m\": " << e->getLength()
          << ", \"primary_tube_length_m\": " << e->getPrimaryTubeLength()
          << ", \"collector_area_m2\": " << e->getCollectorCrossSectionArea()
          << ", \"audio_volume\": " << e->getAudioVolume() << ", \"velocity_decay\": " << e->getVelocityDecay()
          << ", \"impulse_response\": " << jsonString(fs::path(ir->getFilename()).filename().string())
          << ", \"impulse_response_volume\": " << ir->getVolume() << " }";
    }
    f << "\n  ],\n  \"cylinders\": [";
    for (int i = 0; i < engine->getCylinderCount(); ++i) {
        Piston *p = engine->getChamber(i)->getPiston();
        CylinderHead *h = engine->getChamber(i)->getCylinderHead();
        ConnectingRod *r = p->getRod();
        // A slave rod of a radial sits on a journal of its master rod, not of the crankshaft.
        ConnectingRod *m = r->getMasterRod();
        Crankshaft *rc = (m != nullptr ? m : r)->getCrankshaft();
        const int local = p->getCylinderIndex();
        int crank = 0;
        for (int k = 0; k < engine->getCrankshaftCount(); ++k) {
            if (engine->getCrankshaft(k) == rc) crank = k;
        }
        int intake = -1;
        for (int k = 0; k < engine->getIntakeCount(); ++k) {
            if (engine->getIntake(k) == h->getIntake(local)) intake = k;
        }
        const double bore = p->getCylinderBank()->getBore();
        const double stroke = 2 * rc->getThrow();
        const double swept = M_PI / 4 * bore * bore * stroke;
        const double clearance = h->getCombustionChamberVolume();
        f << (i ? "," : "") << "\n    { \"bank\": " << p->getCylinderBank()->getIndex() << ", \"bank_position\": " << local
          << ", \"crankshaft\": " << crank << ", \"master_rod\": " << (m != nullptr ? "true" : "false")
          << ", \"journal\": " << (m != nullptr ? m : r)->getJournal()
          << ", \"journal_deg\": " << deg(rc->getRodJournalAngle((m != nullptr ? m : r)->getJournal()))
          << ", \"spark_deg\": " << deg(ign->getSparkAngle(i)) << ", \"spark_enabled\": " << (ign->getSparkEnabled(i) ? "true" : "false")
          << ", \"bore_m\": " << bore << ", \"stroke_m\": " << stroke << ", \"rod_m\": " << r->getLength()
          << ", \"compression_ratio\": " << (clearance > 0 ? (swept + clearance) / clearance : 0.0)
          << ", \"exhaust_system\": " << h->getExhaustSystem(local)->getIndex() << ", \"intake\": " << intake
          << ", \"header_primary_length_m\": " << h->getHeaderPrimaryLength(local)
          << ", \"sound_attenuation\": " << h->getSoundAttenuation(local)
          << ", \"exhaust_runner_area_m2\": " << h->getExhaustRunnerCrossSectionArea()
          << ", \"exhaust_runner_volume_m3\": " << h->getExhaustRunnerVolume()
          << ", \"intake_runner_area_m2\": " << h->getIntakeRunnerCrossSectionArea() << " }";
    }
    f << "\n  ]\n}\n";
    if (!f) die("write failed: " + path.string());
}

// The --dump-synth record. input_N.f64 holds the input of channel N, one
// little-endian float64 per simulation step, as
// PistonEngineSimulator::writeToSynthesizer passes it to
// Synthesizer::writeInput. Each frame records its step count and its render
// blocks: the samples that Synthesizer::renderAudio() rendered, and the
// rand() calls outside the synthesiser since the previous block.
struct SynthDump {
    struct Channel {
        std::string impulseResponse;
        std::string impulseResponsePath;
        size_t fileSamples = 0;
        int taps = 0;
        double volume = 0;
    };
    fs::path dir;
    std::vector<std::ofstream> inputs;
    std::vector<Channel> channels;
    std::ostringstream frames;
    long long randMark = 0;
    long long frameCount = 0;
};

void writeSynthDump(SynthDump &d, Synthesizer &synth, Engine *engine, const fs::path &script, bool noNoise,
                    const std::vector<int16_t> &rendered, size_t settleSamples, size_t keptSamples) {
    for (std::ofstream &in : d.inputs) {
        in.close();
        if (!in) die("write failed in " + d.dir.string());
    }
    writeWav16((d.dir / "output.wav").string(), std::vector<float>(rendered.begin(), rendered.end()), SynthRate);

    const Synthesizer::AudioParameters a = synth.getAudioParameters();
    std::ofstream f(d.dir / "synth.json");
    if (!f) die("cannot write " + (d.dir / "synth.json").string());
    f.precision(17);
    f << "{\n  \"engine\": " << jsonString(engine->getName())
      << ",\n  \"script\": " << jsonString(script.filename().string())
      << ",\n  \"no_noise\": " << (noNoise ? "true" : "false")
      << ",\n  \"simulation_frequency_hz\": " << engine->getSimulationFrequency()
      << ",\n  \"input_sample_rate_hz\": " << synth.getInputSampleRate()
      << ",\n  \"audio_sample_rate_hz\": " << synth.m_audioSampleRate
      << ",\n  \"input_channel_count\": " << synth.m_inputChannelCount
      << ",\n  \"input_buffer_size\": " << synth.m_inputBufferSize
      << ",\n  \"audio_buffer_size\": " << synth.m_audioBufferSize
      << ",\n  \"audio_parameters\": { \"volume\": " << a.volume << ", \"convolution\": " << a.convolution
      << ", \"dF_F_mix\": " << a.dF_F_mix << ", \"inputSampleNoise\": " << a.inputSampleNoise
      << ", \"inputSampleNoiseFrequencyCutoff\": " << a.inputSampleNoiseFrequencyCutoff
      << ", \"airNoise\": " << a.airNoise << ", \"airNoiseFrequencyCutoff\": " << a.airNoiseFrequencyCutoff
      << ", \"levelerTarget\": " << a.levelerTarget << ", \"levelerMaxGain\": " << a.levelerMaxGain
      << ", \"levelerMinGain\": " << a.levelerMinGain << " }"
      << ",\n  \"leveling_filter\": { \"p_target\": " << synth.m_levelingFilter.p_target
      << ", \"p_maxLevel\": " << synth.m_levelingFilter.p_maxLevel
      << ", \"p_minLevel\": " << synth.m_levelingFilter.p_minLevel << " }"
      << ",\n  \"derivative_dt\": " << synth.m_filters[0].derivative.m_dt
      << ",\n  \"input_dc_filter_dt\": " << synth.m_filters[0].inputDcFilter.m_dt
      << ",\n  \"channels\": [";
    for (size_t i = 0; i < d.channels.size(); ++i) {
        const SynthDump::Channel &c = d.channels[i];
        f << (i ? "," : "") << "\n    { \"input\": \"input_" << i << ".f64\", \"impulse_response\": "
          << jsonString(c.impulseResponse) << ", \"impulse_response_path\": " << jsonString(c.impulseResponsePath)
          << ", \"impulse_response_copy\": \"ir_" << i << ".wav\", \"impulse_response_file_samples\": " << c.fileSamples
          << ", \"impulse_response_taps\": " << c.taps << ", \"impulse_response_volume\": " << c.volume << " }";
    }
    f << "\n  ],\n  \"output\": \"output.wav\", \"output_samples\": " << rendered.size()
      << ", \"settle_samples\": " << settleSamples << ", \"kept_samples\": " << keptSamples
      << ",\n  \"frames\": [" << d.frames.str() << "\n  ]\n}\n";
    if (!f) die("write failed: " + (d.dir / "synth.json").string());
}

} // namespace

int main(int argc, char **argv) {
    const Options opt = parseArgs(argc, argv);

    const fs::path script = fs::absolute(opt.script);
    const fs::path out = fs::absolute(opt.out);
    const fs::path csvPath = opt.csv.empty() ? fs::path() : fs::absolute(opt.csv);
    const fs::path dumpDir = opt.dumpSynth.empty() ? fs::path() : fs::absolute(opt.dumpSynth);
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
    if (opt.noNoise) {
        audio.inputSampleNoise = 0;
        audio.airNoise = 0;
    }
    synth.setAudioParameters(audio);

    SynthDump dump;
    if (!opt.dumpSynth.empty()) {
        dump.dir = dumpDir;
        fs::create_directories(dump.dir);
    }

    for (int i = 0; i < engine->getExhaustSystemCount(); ++i) {
        ImpulseResponse *ir = engine->getExhaustSystem(i)->getImpulseResponse();
        fs::path irPath = ir->getFilename();
        if (irPath.is_relative()) irPath = work / irPath;
        const std::vector<int16_t> samples = readWav16(irPath.string());
        synth.initializeImpulseResponse(samples.data(), static_cast<unsigned int>(samples.size()),
            static_cast<float>(ir->getVolume()), i);
        if (!opt.dumpSynth.empty()) {
            const fs::path file = fs::canonical(irPath);
            const fs::path copy = dump.dir / ("ir_" + std::to_string(i) + ".wav");
            // The package files are read-only, so the copy of an earlier dump is too.
            fs::remove(copy);
            fs::copy_file(file, copy);
            fs::permissions(copy, fs::perms::owner_write, fs::perm_options::add);
            dump.channels.push_back({ fs::relative(file, fs::canonical(work / "es")).string(), file.string(),
                samples.size(), synth.m_filters[i].convolution.getSampleCount(), static_cast<float>(ir->getVolume()) });
            dump.inputs.emplace_back(dump.dir / ("input_" + std::to_string(i) + ".f64"), std::ios::binary);
            if (!dump.inputs.back()) die("cannot write in " + dump.dir.string());
        }
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
    double idleSum = 0;
    bool caught = false;
    long long idleSteps = 0;

    while (t < endTime) {
        const double segT = t - opt.settle;
        const Control c = controlAt(opt.segments, std::max(0.0, segT));
        // The starter cranks for the first half second of the pre-roll. In
        // the idle run it cranks until the engine runs at twice the starter
        // speed, for at most 3 s, as a driver holds the key.
        if (opt.dumpJson.empty()) sim->m_starterMotor.m_enabled = t < 0.5;
        else {
            caught = caught || engine->getRpm() > 2 * std::abs(units::toRpm(engine->getStarterSpeed())) || t >= 3.0;
            sim->m_starterMotor.m_enabled = !caught;
        }
        sim->m_dyno.m_enabled = c.dyno && t >= 0.5;
        sim->m_dyno.m_rotationSpeed = units::rpm(c.rpm);
        engine->setSpeedControl(c.throttle);

        sim->startFrame(frame);
        long long frameSteps = 0;
        while (sim->simulateStep()) {
            ++step;
            ++frameSteps;
            if (!opt.dumpSynth.empty()) {
                const double *in = static_cast<PistonEngineSimulator *>(sim)->getExhaustFlowStagingBuffer();
                for (size_t i = 0; i < dump.inputs.size(); ++i) {
                    dump.inputs[i].write(reinterpret_cast<const char *>(&in[i]), sizeof(double));
                }
            }
            if (step * timestep > endTime - 2.0) {
                idleSum += engine->getRpm();
                ++idleSteps;
            }
            if (csv.is_open() && step % opt.csvEvery == 0) {
                csv << step * timestep - opt.settle << ',' << engine->getRpm() << ',' << c.throttle
                    << ',' << sim->m_dyno.getTorque();
                for (int i = 0; i < cylinders; ++i) csv << ',' << engine->getChamber(i)->m_exhaustRunnerAndPrimary.pressure();
                for (int i = 0; i < cylinders; ++i) csv << ',' << engine->getChamber(i)->getLastTimestepExhaustFlow();
                csv << '\n';
            }
        }
        sim->endFrame();

        if (!opt.dumpSynth.empty()) {
            dump.frames << (dump.frameCount++ ? ",\n    " : "\n    ") << "{ \"steps\": " << frameSteps << ", \"blocks\": [";
        }
        int blocks = 0;
        while (synth.m_inputChannels[0].data.size() > 0) {
            const long long randBefore = g_randCalls;
            synth.renderAudio();
            const int n = static_cast<int>(synth.m_audioBuffer.size());
            if (!opt.dumpSynth.empty()) {
                // Synthesizer::renderAudio(int) calls rand() twice per channel and sample.
                if (g_randCalls - randBefore != 2LL * synth.m_inputChannelCount * n) {
                    die("unexpected rand() calls in a synthesiser block");
                }
                dump.frames << (blocks++ ? ", " : "") << "[" << n << ", " << randBefore - dump.randMark << "]";
                dump.randMark = g_randCalls;
            }
            if (n > static_cast<int>(chunk.size())) chunk.resize(n);
            synth.readAudioOutput(n, chunk.data());
            rendered.insert(rendered.end(), chunk.begin(), chunk.begin() + n);
            synth.endInputBlock();
        }
        if (!opt.dumpSynth.empty()) dump.frames << "] }";

        t = step * timestep;
        if (t <= opt.settle) settleSamples = rendered.size();
    }

    if (!opt.dumpJson.empty()) {
        writeDump(out, engine, idleSteps > 0 ? idleSum / idleSteps : 0.0, opt.idleSeconds);
        std::cerr << "engine-sim-render: " << engine->getName() << ", " << cylinders << " cylinders, idle "
                  << std::lround(idleSteps > 0 ? idleSum / idleSteps : 0.0) << " rpm, wrote " << out.string() << "\n";
        sim->releaseSimulation();
        delete sim;
        return 0;
    }

    const size_t want = static_cast<size_t>(std::llround(total * SynthRate));
    const size_t start = std::min(settleSamples, rendered.size());
    std::vector<int16_t> kept(rendered.begin() + start, rendered.begin() + std::min(rendered.size(), start + want));
    writeWav16(out.string(), resample(kept, SynthRate, opt.rate), opt.rate);
    if (!opt.dumpSynth.empty()) writeSynthDump(dump, synth, engine, script, opt.noNoise, rendered, start, kept.size());

    std::cerr << "engine-sim-render: " << engine->getName() << ", " << cylinders << " cylinders, "
              << engine->getExhaustSystemCount() << " exhaust systems, sim " << sim->getSimulationFrequency()
              << " Hz, final " << std::lround(engine->getRpm()) << " rpm, wrote "
              << double(kept.size()) / SynthRate << " s to " << out.string() << "\n";

    sim->releaseSimulation();
    delete sim;
    return 0;
}
