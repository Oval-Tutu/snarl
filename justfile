# Requires just 1.23.0 or later.
#
# Tailor fitted this justfile and manages the recipes in just/*.just, which
# give build, test, and lint. The recipes below belong to the project; tailor
# leaves them alone.
import 'just/loader.just'

# List available recipes
default:
    @just --list

# Scan the module for known vulnerabilities.
vuln:
    govulncheck -show verbose ./...

# Run the real-time Ebitengine harness, with pad input and an on-screen
# display of RPM, throttle, load, and gear.
run-harness *args: (_command "snarl-harness")
    cd cmd/snarl-harness && go run . {{ args }}

# Vet and test the harness, which is a separate module.
test-harness: (_command "snarl-harness")
    cd cmd/snarl-harness && go vet ./... && go test -cover -covermode=atomic ./...

# Lint the harness module with the root golangci-lint configuration.
lint-harness: (_command "snarl-harness")
    cd cmd/snarl-harness && go mod tidy -diff && golangci-lint fmt --config ../../.golangci.yml --diff && golangci-lint run --config ../../.golangci.yml

# Render a profile offline to a WAV file from an RPM and throttle script.
render profile script *args: (_command "snarl-render")
    go run ./cmd/snarl-render {{ args }} "{{ profile }}" "{{ script }}"

# Compare two renders and write metrics and spectrograms.
compare a b *args: (_command "snarl-compare")
    go run ./cmd/snarl-compare {{ args }} "{{ a }}" "{{ b }}"

# Analyse one render that a scenario drove, and write the results to the
# out directory: spectrogram.png (log frequency and magnitude),
# waveform.png (linear peak amplitude), waveform-log.png (logarithmic
# amplitude), histogram.png (sample value histogram over the whole file),
# the ffmpeg logs ebur128.log (momentary and short-term loudness every
# 100 ms), astats.log (RMS, peak, crest factor and noise floor every
# 100 ms) and aspectralstats.log (centroid, flatness, rolloff and flux per
# 4096-sample frame), and the snarl-compare tables describe.md (level,
# spectral shape, bands and orders per window) and residual.md (residual
# of each hold).
analyse wav scenario out: (_command "snarl-compare")
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    mkdir -p "{{ out }}"
    ff=(ffmpeg -nostdin -hide_banner -loglevel error -y -i "{{ wav }}")
    "${ff[@]}" -lavfi "showspectrumpic=s=1600x600:scale=log:fscale=log:legend=1" "{{ out }}/spectrogram.png"
    "${ff[@]}" -filter_complex "showwavespic=s=1600x400:scale=lin:filter=peak:colors=white[w];color=c=black:s=1600x400[b];[b][w]overlay=format=auto,format=rgb24" -frames:v 1 "{{ out }}/waveform.png"
    "${ff[@]}" -filter_complex "showwavespic=s=1600x400:scale=log:filter=peak:colors=white[w];color=c=black:s=1600x400[b];[b][w]overlay=format=auto,format=rgb24" -frames:v 1 "{{ out }}/waveform-log.png"
    "${ff[@]}" -lavfi "ahistogram=s=800x400:acount=-1:rheight=1:hmode=sign:scale=log:ascale=lin,format=rgb24" -update 1 "{{ out }}/histogram.png"
    "${ff[@]}" -af "ebur128=metadata=1,ametadata=mode=print:file={{ out }}/ebur128.log" -f null -
    "${ff[@]}" -af "asetnsamples=n=4800,astats=metadata=1:reset=1:measure_overall=none:measure_perchannel=RMS_level+Peak_level+Crest_factor+Noise_floor,ametadata=mode=print:file={{ out }}/astats.log" -f null -
    "${ff[@]}" -af "aspectralstats=win_size=4096:measure=centroid+flatness+rolloff+flux,ametadata=mode=print:file={{ out }}/aspectralstats.log" -f null -
    go run ./cmd/snarl-compare --describe --scenario "{{ scenario }}" "{{ wav }}" > "{{ out }}/describe.md"
    go run ./cmd/snarl-compare --residual --scenario "{{ scenario }}" "{{ wav }}" > "{{ out }}/residual.md"
    echo "Wrote the analysis of {{ wav }} to {{ out }}"

# Render an engine-sim reference to a WAV file. The script is a path, or a
# path inside the upstream engines directory. Pass scenario segments as args,
# for example: just ref-engine-sim atg-video-2/07_gm_ls.mr out/v8.wav --hold 3000:0.5:3 --volume 0.5
ref-engine-sim script out *args:
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build .#engine-sim-render --print-out-paths --no-link)"
    script="{{ script }}"
    if [[ ! -f "${script}" ]]; then
        script="${r}/share/engine-sim/engines/${script}"
    fi
    mkdir -p "$(dirname "{{ out }}")"
    "${r}/bin/engine-sim-render" --script "${script}" --out "{{ out }}" {{ args }}

# Render every scenario of the profile catalogue with engine-sim, for one
# engine-sim engine of the catalogue (catalogue/<id>) or for all of them
# when none is given (scenarios/catalogue/engine-sim-scripts.txt). Writes
# out/reference/engine-sim/catalogue/<id>/<scenario>.wav with a CSV beside
# it. just ref-upstream renders the references of the embedded profiles.
ref-set profile="catalogue":
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build -f reference/engine-sim --print-out-paths --no-link)"
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT
    declare -A scripts=()
    profiles=({{ profile }})
    catalogue=scenarios/catalogue/engine-sim-scripts.txt
    if [[ "${profiles[*]}" == catalogue ]]; then
        mapfile -t profiles < <(sed -e 's/#.*//' "${catalogue}" | awk 'NF { print "catalogue/" $1 }')
    fi
    for p in "${profiles[@]}"; do
        if [[ "${p}" == catalogue/* ]]; then
            rel="$(sed -e 's/#.*//' "${catalogue}" | awk -v id="${p#catalogue/}" '$1 == id { print $2 }')"
            if [[ -n "${rel}" ]]; then
                scripts[${p}]="$(tools/import/engine-sim-main.sh "${r}/share/engine-sim/engines/${rel}" "${tmp}")"
            fi
        fi
        if [[ -z "${scripts[${p}]:-}" ]]; then
            echo "Error: unknown profile '${p}'. Use catalogue or catalogue/<id> of an engine-sim engine." >&2
            exit 1
        fi
        out="out/reference/engine-sim/${p}"
        mkdir -p "${out}"
        for scn in scenarios/"${p}"/*.scn; do
            name="$(basename "${scn}" .scn)"
            segments=()
            while read -r kind a b c d extra; do
                case "${kind}" in
                    "") continue ;;
                    hold) [[ -n "${c}" && -z "${d}" ]] && segments+=(--hold "${a}:${b}:${c}") && continue ;;
                    sweep) [[ -n "${d}" && -z "${extra}" ]] && segments+=(--sweep "${a}:${b}:${c}:${d}") && continue ;;
                esac
                echo "Error: ${scn}: bad segment '${kind} ${a} ${b} ${c} ${d} ${extra}'" >&2
                exit 1
            done < <(sed -e 's/#.*//' "${scn}")
            "${r}/bin/engine-sim-render" --script "${scripts[${p}]}" "${segments[@]}" \
                --volume 0.5 --out "${out}/${name}.wav" --csv "${out}/${name}.csv" --csv-every 100
        done
    done

# Render the references of five upstream engine-sim engines, unchanged, for
# one engine or for all when none is given. The engine names map to the
# upstream scripts in the pinned package (scenarios/upstream/README.md):
# i4-vtec atg-video-1/05_honda_vtec.mr, v8-ls atg-video-2/07_gm_ls.mr,
# f4-ej25 atg-video-1/06_subaru_ej25.mr, v12-412
# atg-video-2/12_ferrari_412_t2.mr and v12-merlin atg-video-2/11_merlin_v12.mr.
# For each engine it writes to out/reference/upstream/<engine>: the engine
# dump engine.json, the scenarios that tools/fit --upstream-scenarios
# writes from it into scenarios/upstream/<engine>, a WAV and a CSV of each
# scenario at --volume 0.5 (every step for the holds and the free runs,
# every 10th step for sweep-wot and overrun), and health.md from tools/fit
# --health. The simulation-rate check renders the simrate hold again from a
# temporary copy of the script with only the simulation frequency doubled,
# as simrate-2x, and compares the two in simrate-compare with snarl-compare.
ref-upstream engine="":
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build -f reference/engine-sim --print-out-paths --no-link)"
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT
    declare -A scripts=(
        [i4-vtec]=atg-video-1/05_honda_vtec.mr
        [v8-ls]=atg-video-2/07_gm_ls.mr
        [f4-ej25]=atg-video-1/06_subaru_ej25.mr
        [v12-412]=atg-video-2/12_ferrari_412_t2.mr
        [v12-merlin]=atg-video-2/11_merlin_v12.mr
    )
    # Sets segments to the renderer flags of the scenario file $1.
    parse() {
        segments=()
        while IFS= read -r line; do
            read -r kind a b c d extra <<< "${line%%#*}" || true
            case "${kind:-}" in
                "") continue ;;
                hold) [[ -n "${c}" && -z "${d}" ]] && segments+=(--hold "${a}:${b}:${c}") && continue ;;
                sweep) [[ -n "${d}" && -z "${extra}" ]] && segments+=(--sweep "${a}:${b}:${c}:${d}") && continue ;;
                free) [[ -n "${b}" && -z "${c}" ]] && segments+=(--free "${a}:${b}") && continue ;;
            esac
            echo "Error: $1: bad segment '${line}'" >&2
            exit 1
        done < "$1"
    }
    engines=({{ if engine == "" { "i4-vtec v8-ls f4-ej25 v12-412 v12-merlin" } else { engine } }})
    for e in "${engines[@]}"; do
        if [[ -z "${scripts[${e}]:-}" ]]; then
            echo "Error: unknown engine '${e}'. Use i4-vtec, v8-ls, f4-ej25, v12-412 or v12-merlin." >&2
            exit 1
        fi
        # A script with a main node is used as it is. A script without one
        # gets a wrapper that only adds main.
        script="$(tools/import/engine-sim-main.sh "${r}/share/engine-sim/engines/${scripts[${e}]}" "${tmp}")"
        out="out/reference/upstream/${e}"
        scn="scenarios/upstream/${e}"
        mkdir -p "${out}"
        echo "${e}: ${scripts[${e}]} (${script})"
        "${r}/bin/engine-sim-render" --script "${script}" --dump-json "${out}/engine.json"
        go run ./tools/fit --upstream-scenarios --engine "${out}/engine.json" --scenarios "${scn}" --name "${e}"
        for f in "${scn}"/*.scn; do
            name="$(basename "${f}" .scn)"
            [[ "${name}" == simrate-2x ]] && continue
            every=1
            [[ "${name}" == sweep-wot || "${name}" == overrun ]] && every=10
            parse "${f}"
            "${r}/bin/engine-sim-render" --script "${script}" "${segments[@]}" \
                --volume 0.5 --out "${out}/${name}.wav" --csv "${out}/${name}.csv" --csv-every "${every}"
        done
        # The simulation-rate check: a copy of the script with only the
        # simulation frequency literal doubled.
        hz="$(grep -o '"simulation_frequency_hz": [0-9]*' "${out}/engine.json")"
        hz="${hz##* }"
        if [[ "$(grep -cE "simulation_frequency: ${hz}([^0-9.]|$)" "${script}")" != 1 ]]; then
            echo "Error: ${script} does not set simulation_frequency: ${hz} once." >&2
            exit 1
        fi
        text="$(< "${script}")"
        printf '%s\n' "${text/"simulation_frequency: ${hz}"/"simulation_frequency: $((hz * 2))"}" > "${tmp}/${e}-2x.mr"
        parse "${scn}/simrate-2x.scn"
        "${r}/bin/engine-sim-render" --script "${tmp}/${e}-2x.mr" "${segments[@]}" \
            --volume 0.5 --out "${out}/simrate-2x.wav" --csv "${out}/simrate-2x.csv"
        go run ./tools/fit --health --engine "${out}/engine.json" --scenarios "${scn}" --name "${e}" \
            "${out}"/holds-t000.csv "${out}"/holds-t050.csv "${out}"/holds-t100.csv "${out}"/idle-free.csv \
            "${out}"/limiter-free.csv "${out}"/simrate.csv "${out}"/simrate-2x.csv > "${out}/health.md"
        go run ./cmd/snarl-compare --scenario "${scn}/simrate.scn" --out "${out}/simrate-compare" \
            "${out}/simrate.wav" "${out}/simrate-2x.wav"
        echo "Wrote ${out}"
    done

# Build the cycle tables of the five upstream engine-sim engines
# (docs/research/cycle-tables.md), for one engine or for all when none is
# given. tools/cycles scenarios writes the grid scenarios into
# scenarios/upstream/<engine>/grid from the first and last hold of
# holds-t100.scn: one file per throttle row (0, 0.25, 0.5, 0.75 and 1) with
# a hold at idle, every multiple of the RPM step and the top, cut.scn
# with ignition-off holds, and step.scn with throttle steps for the intake
# lag. The renderer runs each file with --dump-synth into
# out/cycles/grid/<engine>/<file>/, in parallel, from the unchanged
# upstream script at its own simulation frequency. tools/cycles build then
# writes profiles/cycles/<engine>.bin and its report build.md.
cycles-grid engine="":
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build -f reference/engine-sim --print-out-paths --no-link)"
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT
    declare -A scripts=(
        [i4-vtec]=atg-video-1/05_honda_vtec.mr
        [v8-ls]=atg-video-2/07_gm_ls.mr
        [f4-ej25]=atg-video-1/06_subaru_ej25.mr
        [v12-412]=atg-video-2/12_ferrari_412_t2.mr
        [v12-merlin]=atg-video-2/11_merlin_v12.mr
    )
    # The RPM step of the grid of each engine.
    declare -A steps=(
        [i4-vtec]=250
        [v8-ls]=250
        [f4-ej25]=250
        [v12-412]=250
        [v12-merlin]=250
    )
    # Sets segments to the renderer flags of the grid scenario file $1.
    parse() {
        segments=()
        while IFS= read -r line; do
            read -r kind a b c d extra <<< "${line%%#*}" || true
            case "${kind:-}" in
                "") continue ;;
                hold) [[ -n "${c}" && -z "${d}" ]] && segments+=(--hold "${a}:${b}:${c}") && continue ;;
                sweep) [[ -n "${d}" && -z "${extra}" ]] && segments+=(--sweep "${a}:${b}:${c}:${d}") && continue ;;
                cut) [[ -n "${c}" && -z "${d}" ]] && segments+=(--cut "${a}:${b}:${c}") && continue ;;
            esac
            echo "Error: $1: bad segment '${line}'" >&2
            exit 1
        done < "$1"
    }
    engines=({{ if engine == "" { "i4-vtec v8-ls f4-ej25 v12-412 v12-merlin" } else { engine } }})
    for e in "${engines[@]}"; do
        if [[ -z "${scripts[${e}]:-}" ]]; then
            echo "Error: unknown engine '${e}'. Use i4-vtec, v8-ls, f4-ej25, v12-412 or v12-merlin." >&2
            exit 1
        fi
        script="$(tools/import/engine-sim-main.sh "${r}/share/engine-sim/engines/${scripts[${e}]}" "${tmp}")"
        scn="scenarios/upstream/${e}/grid"
        out="out/cycles/grid/${e}"
        go run ./tools/cycles scenarios --holds "scenarios/upstream/${e}/holds-t100.scn" --name "${e}" \
            --step "${steps[${e}]}" --out "${scn}"
        rm -rf "${out}"
        mkdir -p "${out}"
        pids=()
        for f in "${scn}"/*.scn; do
            name="$(basename "${f}" .scn)"
            parse "${f}"
            "${r}/bin/engine-sim-render" --script "${script}" "${segments[@]}" --volume 0.5 --rate 44100 \
                --out "${out}/${name}.wav" --dump-synth "${out}/${name}" > "${out}/${name}.log" 2>&1 &
            pids+=($!)
        done
        for p in "${pids[@]}"; do
            wait "${p}"
        done
        mkdir -p profiles/cycles
        go run ./tools/cycles build --out "profiles/cycles/${e}.bin" --step "${out}/step" \
            "${out}"/t*/ "${out}/cut" > "${out}/build.md"
        tail -n 1 "${out}/build.md"
    done

# Render the EngineLab references of the Aircooled-like 3.6 Flat-6 with the
# EngineLab offline audio exporter (reference/enginelab/README.md). For each
# scenario in reference/enginelab/scenarios it writes the exporter output
# (master, stems, engine order map and manifest) to
# out/reference/enginelab/f6-aircooled/<scenario>/ at 48 kHz, 32-bit float
# and --volume 0.5, and <scenario>.wav beside it: the master without the
# pre-roll stages, so that it matches scenarios/upstream/f6-aircooled.
ref-enginelab:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    r="$(nix build -f reference/enginelab --print-out-paths --no-link)"
    jq="$(nix build --inputs-from . nixpkgs#jq.bin --print-out-paths --no-link)/bin/jq"
    out=out/reference/enginelab/f6-aircooled
    mkdir -p "${out}"
    for f in reference/enginelab/scenarios/*.json; do
        name="$(basename "${f}" .json)"
        "${r}/bin/EngineLabOfflineAudioExporter" --catalog-root "${r}/share/enginelab" \
            --engine "Aircooled-like 3.6 Flat-6" --scenario "${f}" --output "${out}/${name}" \
            --sample-rate 48000 --format float32 --volume 0.5 --stems --overwrite
        preroll="$("${jq}" '[.stages[] | select(.name | startswith("preroll_")) | .duration_seconds] | add // 0' "${f}")"
        ffmpeg -nostdin -hide_banner -loglevel error -y -i "${out}/${name}/master.wav" -ss "${preroll}" \
            -c:a pcm_f32le "${out}/${name}.wav"
        echo "Wrote ${out}/${name}.wav (${preroll} s pre-roll removed)"
    done

# Build listening files in out/listen from existing snarl renders and the
# upstream references of the six embedded profiles: each snarl render
# out/snarl/<profile>/<scenario>.wav followed by its reference, and blind
# sets of the snarl sweep-wot and overrun renders in a new random order with
# the answer in key.txt. The references are out/reference/upstream/<profile>
# of just ref-upstream, and out/reference/enginelab/f6-aircooled of just
# ref-enginelab. All parts are loudness-normalised with loudnorm.
listen:
    #!/usr/bin/env bash
    set -euo pipefail
    profiles=(i4-vtec v8-ls f4-ej25 v12-412 v12-merlin f6-aircooled)
    scenarios=(sweep-wot overrun holds-t100)
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    # Prints the reference of profile $1 and scenario $2.
    ref() {
        if [[ "$1" == f6-aircooled ]]; then
            echo "out/reference/enginelab/$1/$2.wav"
        else
            echo "out/reference/upstream/$1/$2.wav"
        fi
    }
    missing=()
    for p in "${profiles[@]}"; do
        for s in "${scenarios[@]}"; do
            for f in "out/snarl/${p}/${s}.wav" "$(ref "${p}" "${s}")"; do
                [[ -f "${f}" ]] || missing+=("${f}")
            done
        done
    done
    if (( ${#missing[@]} > 0 )); then
        echo "Error: missing renders, so out/listen is unchanged:" >&2
        printf '  %s\n' "${missing[@]}" >&2
        echo "Render them with just render, just ref-upstream and just ref-enginelab." >&2
        exit 1
    fi
    norm="loudnorm=I=-16:TP=-1.5:LRA=11,aresample=48000"
    ff=(ffmpeg -nostdin -hide_banner -loglevel error -y)
    rm -rf out/listen
    mkdir -p out/listen/blind out/listen/blind-overrun
    for p in "${profiles[@]}"; do
        for s in "${scenarios[@]}"; do
            out="out/listen/${p}-${s}-snarl-then-reference.wav"
            "${ff[@]}" -i "out/snarl/${p}/${s}.wav" -i "$(ref "${p}" "${s}")" \
                -filter_complex "[0:a]${norm}[a];[1:a]${norm}[b];[a][b]concat=n=2:v=0:a=1" \
                -c:a pcm_s16le "${out}"
            echo "Wrote ${out}"
        done
    done
    for set in blind:sweep-wot blind-overrun:overrun; do
        dir="out/listen/${set%%:*}"
        s="${set#*:}"
        n=0
        for p in $(printf '%s\n' "${profiles[@]}" | shuf); do
            n=$((n + 1))
            "${ff[@]}" -i "out/snarl/${p}/${s}.wav" -af "${norm}" -c:a pcm_s16le "${dir}/blind-${n}.wav"
            echo "blind-${n} = ${p}" >> "${dir}/key.txt"
        done
        echo "Wrote ${n} blind files and key.txt to ${dir}"
    done

# Build out/listen-overnight/<profile>.wav for the six embedded profiles: the
# sweep-wot render of the snarl-render binary at the path before, the tier 1
# render of the current tree, and the upstream reference, in that order with
# 1 s of silence between them. The binary before must know the six profiles.
# A plain gain takes each part from its measured integrated loudness to
# -16 LUFS. Writes a spectrogram <profile>.png beside each file, and
# README.txt with the start time and the measured loudness of each part.
listen-overnight before: (_listen-overnight before "i4-vtec") (_listen-overnight before "v8-ls") (_listen-overnight before "f4-ej25") (_listen-overnight before "v12-412") (_listen-overnight before "v12-merlin") (_listen-overnight before "f6-aircooled")
    cd out/listen-overnight && cat i4-vtec.line v8-ls.line f4-ej25.line v12-412.line v12-merlin.line f6-aircooled.line > README.txt
    cd out/listen-overnight && rm i4-vtec.line v8-ls.line f4-ej25.line v12-412.line v12-merlin.line f6-aircooled.line
    @echo "Wrote out/listen-overnight"

_listen-overnight before profile: (_command "snarl-render")
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    out=out/listen-overnight
    work="$(mktemp -d)"
    trap 'rm -rf "${work}"' EXIT
    mkdir -p "${out}"
    scn="scenarios/upstream/{{ profile }}/sweep-wot.scn"
    ref="out/reference/upstream/{{ profile }}/sweep-wot.wav"
    if [[ "{{ profile }}" == f6-aircooled ]]; then
        ref="out/reference/enginelab/{{ profile }}/sweep-wot.wav"
    fi
    "{{ before }}" -rate 48000 -format f32 -tier 1 -out "${work}/before.wav" "{{ profile }}" "${scn}"
    go run ./cmd/snarl-render -rate 48000 -format f32 -tier 1 -out "${work}/after.wav" "{{ profile }}" "${scn}"
    lb="$(ffmpeg -nostdin -hide_banner -i "${work}/before.wav" -af ebur128=framelog=quiet -f null - 2>&1 | grep -E '^ +I:')"
    la="$(ffmpeg -nostdin -hide_banner -i "${work}/after.wav" -af ebur128=framelog=quiet -f null - 2>&1 | grep -E '^ +I:')"
    le="$(ffmpeg -nostdin -hide_banner -i "${ref}" -af ebur128=framelog=quiet -f null - 2>&1 | grep -E '^ +I:')"
    lb="${lb#*I:}" la="${la#*I:}" le="${le#*I:}"
    lb="${lb%LUFS}" la="${la%LUFS}" le="${le%LUFS}"
    lb="${lb// /}" la="${la// /}" le="${le// /}"
    nb="$(ffprobe -v error -select_streams a:0 -show_entries stream=duration_ts -of default=nw=1:nk=1 "${work}/before.wav")"
    na="$(ffprobe -v error -select_streams a:0 -show_entries stream=duration_ts -of default=nw=1:nk=1 "${work}/after.wav")"
    fmt="aformat=sample_fmts=flt:sample_rates=48000:channel_layouts=mono"
    # Fade each part in over 10 ms and out over 80 ms, so that the joins do
    # not click.
    fade="afade=t=in:d=0.01,areverse,afade=t=in:d=0.08,areverse"
    ffmpeg -nostdin -hide_banner -loglevel error -y -i "${work}/before.wav" -i "${work}/after.wav" -i "${ref}" \
        -filter_complex "[0:a]${fmt},volume='pow(10,(-16-(${lb}))/20)',${fade}[a];[1:a]${fmt},volume='pow(10,(-16-(${la}))/20)',${fade}[b];[2:a]${fmt},volume='pow(10,(-16-(${le}))/20)',${fade}[c];aevalsrc=0:s=48000:d=1,${fmt}[s1];aevalsrc=0:s=48000:d=1,${fmt}[s2];[a][s1][b][s2][c]concat=n=5:v=0:a=1" \
        -c:a pcm_f32le "${out}/{{ profile }}.wav"
    ffmpeg -nostdin -hide_banner -loglevel error -y -i "${out}/{{ profile }}.wav" -lavfi "showspectrumpic=s=1600x600:scale=log:fscale=log:legend=1" "${out}/{{ profile }}.png"
    ta=$(( (nb + 48000) * 1000 / 48000 ))
    te=$(( (nb + na + 96000) * 1000 / 48000 ))
    printf '{{ profile }}.wav: before at 0.000 s (%s LUFS), after at %d.%03d s (%s LUFS), reference at %d.%03d s (%s LUFS)\n' \
        "${lb}" $(( ta / 1000 )) $(( ta % 1000 )) "${la}" $(( te / 1000 )) $(( te % 1000 )) "${le}" > "${out}/{{ profile }}.line"
    echo "Wrote ${out}/{{ profile }}.wav"

# Stop with an error when the command in cmd/<name> does not exist yet.
_command name:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! -d "cmd/{{ name }}" ]]; then
        echo "Error: cmd/{{ name }} does not exist yet, so this recipe has nothing to run." >&2
        exit 1
    fi

# Import every engine profile of engine-sim and EngineLab into
# profiles/catalogue, with scenarios in scenarios/catalogue
# (profiles/catalogue/README.md). The engine-sim renderer dumps the engine
# that each script builds, yq converts the EngineLab YAML to JSON, and
# tools/import maps both to snarl profiles and calibrates their output
# level. The dumps go to out/import.
import-profiles:
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build -f reference/engine-sim --print-out-paths --no-link)"
    lab="$(nix build -f tools/import/enginelab.nix --print-out-paths --no-link)"
    yq="$(nix build --inputs-from . nixpkgs#yq-go --print-out-paths --no-link)/bin/yq"
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT
    rm -rf out/import
    mkdir -p out/import/engine-sim out/import/enginelab/engines
    engines="${r}/share/engine-sim/engines"
    pids=()
    while read -r rel; do
        script="$(tools/import/engine-sim-main.sh "${engines}/${rel}" "${tmp}")"
        out="out/import/engine-sim/${rel%.mr}.json"
        mkdir -p "$(dirname "${out}")"
        "${r}/bin/engine-sim-render" --script "${script}" --dump-json "${out}" &
        pids+=($!)
    done < <(cd "${engines}" && find . -name '*.mr' ! -name radial.mr | sed 's|^\./||' | sort)
    for pid in "${pids[@]}"; do
        wait "${pid}"
    done
    for f in "${lab}"/engines/*.engine.yaml; do
        "${yq}" -o json "${f}" > "out/import/enginelab/engines/$(basename "${f}" .engine.yaml).json"
    done
    "${yq}" -o json "${lab}/parts/exhausts.yaml" > out/import/enginelab/exhausts.json
    go run ./tools/import --engine-sim out/import/engine-sim --enginelab out/import/enginelab \
        --profiles profiles/catalogue --scenarios scenarios/catalogue

# Rebuild the embedded profiles in profiles (profiles/README.md) from the
# references of just ref-upstream and just ref-enginelab: i4-vtec, v8-ls,
# f4-ej25, v12-412, v12-merlin and f6-aircooled. For each engine-sim engine,
# tools/fit --pulse --variation writes the pulse table to
# internal/model/pulses/<id>.txt and the hold measures to
# out/import/reference/<id>. Then tools/import --reference writes each
# profile with its sources file, and the README. Give id=DIR to use other
# reference renders for an engine, for example v12-412=out/reference/v12-412-10k.
import-reference *refs:
    #!/usr/bin/env bash
    set -euo pipefail
    lab="$(nix build -f tools/import/enginelab.nix --print-out-paths --no-link)"
    yq="$(nix build --inputs-from . nixpkgs#yq-go --print-out-paths --no-link)/bin/yq"
    declare -A ref=(
        [i4-vtec]=out/reference/upstream/i4-vtec
        [v8-ls]=out/reference/upstream/v8-ls
        [f4-ej25]=out/reference/upstream/f4-ej25
        [v12-412]=out/reference/upstream/v12-412
        [v12-merlin]=out/reference/upstream/v12-merlin
        [f6-aircooled]=out/reference/enginelab/f6-aircooled
    )
    flags=()
    for r in {{ refs }}; do
        id="${r%%=*}"
        if [[ "${r}" != *=?* || -z "${ref[${id}]:-}" ]]; then
            echo "Error: '${r}' is not id=DIR of i4-vtec, v8-ls, f4-ej25, v12-412, v12-merlin or f6-aircooled." >&2
            exit 1
        fi
        ref[${id}]="${r#*=}"
        flags+=(--ref "${r}")
    done
    work=out/import/reference
    rm -rf "${work}"
    mkdir -p "${work}/enginelab/engines"
    "${yq}" -o json "${lab}/engines/09_porsche_flat6_like.engine.yaml" > "${work}/enginelab/engines/09_porsche_flat6_like.json"
    "${yq}" -o json "${lab}/parts/exhausts.yaml" > "${work}/enginelab/exhausts.json"
    go build -o "${work}/fit" ./tools/fit
    pids=()
    for id in i4-vtec v8-ls f4-ej25 v12-412 v12-merlin; do
        mkdir -p "${work}/${id}"
        "${work}/fit" --pulse --variation --throttle 1 --engine "${ref[${id}]}/engine.json" \
            --scenarios "scenarios/upstream/${id}" --name "${id}" --out "internal/model/pulses/${id}.txt" \
            --json "${work}/${id}/pulse.json" "${ref[${id}]}/holds-t100.csv" > "${work}/${id}/pulse.md" &
        pids+=($!)
    done
    for pid in "${pids[@]}"; do
        wait "${pid}"
    done
    go run ./tools/import --reference profiles --enginelab "${work}/enginelab" \
        --scenarios scenarios/upstream --work "${work}" "${flags[@]}"
