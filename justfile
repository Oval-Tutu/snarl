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

# Render every scenario in scenarios/<profile> with engine-sim, for one
# profile or for all profiles when none is given. Writes
# out/reference/engine-sim/<profile>/<scenario>.wav with a CSV beside it.
# The profile catalogue/<id> renders a catalogue engine from engine-sim,
# and catalogue renders all of them (scenarios/catalogue/engine-sim-scripts.txt).
ref-set profile="":
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build -f reference/engine-sim --print-out-paths --no-link)"
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT
    declare -A scripts=(
        [flat12]="${PWD}/reference/engine-sim/engines/flat12.mr"
        [inline4]="${r}/share/engine-sim/engines/atg-video-1/05_honda_vtec.mr"
        [flat6]="${PWD}/reference/engine-sim/engines/flat6.mr"
        [v8]="${PWD}/reference/engine-sim/engines/v8.mr"
    )
    profiles=({{ if profile == "" { "flat12 inline4 flat6 v8" } else { profile } }})
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
            echo "Error: unknown profile '${p}'. Use flat12, inline4, flat6, v8, catalogue or catalogue/<id> of an engine-sim engine." >&2
            exit 1
        fi
        out="out/reference/engine-sim/${p}"
        mkdir -p "${out}"
        for scn in scenarios/"${p}"/*.scn; do
            name="$(basename "${scn}" .scn)"
            # A dynamometer hold at the limiter gives no bounce: ref-limiter
            # renders this reference with the engine running free.
            [[ "${name}" == limiter ]] && continue
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

# Render the engine-sim limiter reference for one profile, or for all
# profiles when none is given. The first sweep in scenarios/<profile>/limiter.scn
# gives the start and limiter rpm. The dynamometer holds the start rpm at full
# throttle for 0.5 s, then the engine runs free at full throttle for 5 s and
# bounces off its limiter. The inline4 run uses a copy of the upstream script
# with the rev limit changed from 9400 to the 8500 rpm of the snarl profile.
# Writes out/reference/engine-sim/<profile>/limiter.wav with a CSV beside it.
ref-limiter profile="":
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build .#engine-sim-render --print-out-paths --no-link)"
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT
    honda="${r}/share/engine-sim/engines/atg-video-1/05_honda_vtec.mr"
    sed -e 's/rev_limit: 9400 \* units.rpm/rev_limit: 8500 * units.rpm/' "${honda}" > "${tmp}/inline4.mr"
    if ! grep -q 'rev_limit: 8500 \* units.rpm' "${tmp}/inline4.mr"; then
        echo "Error: no 9400 rpm rev limit in ${honda}." >&2
        exit 1
    fi
    declare -A scripts=(
        [flat12]="${PWD}/reference/engine-sim/engines/flat12.mr"
        [inline4]="${tmp}/inline4.mr"
        [flat6]="${PWD}/reference/engine-sim/engines/flat6.mr"
        [v8]="${PWD}/reference/engine-sim/engines/v8.mr"
    )
    profiles=({{ if profile == "" { "flat12 inline4 flat6 v8" } else { profile } }})
    for p in "${profiles[@]}"; do
        if [[ -z "${scripts[${p}]:-}" ]]; then
            echo "Error: unknown profile '${p}'. Use flat12, inline4, flat6 or v8." >&2
            exit 1
        fi
        scn="scenarios/${p}/limiter.scn"
        read -r kind start limit _ < <(sed -e 's/#.*//' "${scn}" | grep -v '^[[:space:]]*$' | head -n 1)
        if [[ "${kind}" != sweep ]]; then
            echo "Error: ${scn}: the first segment is not a sweep." >&2
            exit 1
        fi
        out="out/reference/engine-sim/${p}"
        mkdir -p "${out}"
        echo "${p}: free run from ${start} rpm into the ${limit} rpm limiter"
        "${r}/bin/engine-sim-render" --script "${scripts[${p}]}" --hold "${start}:1:0.5" --free 1:5 \
            --volume 0.5 --out "${out}/limiter.wav" --csv "${out}/limiter.csv" --csv-every 10
    done

# Build listening files in out/listen from existing snarl and engine-sim
# renders: each snarl render followed by its engine-sim reference, and blind
# sets of the snarl sweep-wot, overrun and limiter renders in a new random
# order with the answer in key.txt. All parts are loudness-normalised with
# loudnorm.
listen:
    #!/usr/bin/env bash
    set -euo pipefail
    profiles=(flat12 inline4 flat6 v8)
    scenarios=(sweep-wot overrun holds-t100 limiter)
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    missing=()
    for p in "${profiles[@]}"; do
        for s in "${scenarios[@]}"; do
            for f in "out/snarl/${p}/${s}.wav" "out/reference/engine-sim/${p}/${s}.wav"; do
                [[ -f "${f}" ]] || missing+=("${f}")
            done
        done
    done
    if (( ${#missing[@]} > 0 )); then
        echo "Error: missing renders, so out/listen is unchanged:" >&2
        printf '  %s\n' "${missing[@]}" >&2
        echo "Render them with just render, just ref-set and just ref-limiter." >&2
        exit 1
    fi
    norm="loudnorm=I=-16:TP=-1.5:LRA=11,aresample=48000"
    ff=(ffmpeg -nostdin -hide_banner -loglevel error -y)
    rm -rf out/listen
    mkdir -p out/listen/blind out/listen/blind-overrun out/listen/blind-limiter
    for p in "${profiles[@]}"; do
        for s in "${scenarios[@]}"; do
            out="out/listen/${p}-${s}-snarl-then-enginesim.wav"
            "${ff[@]}" -i "out/snarl/${p}/${s}.wav" -i "out/reference/engine-sim/${p}/${s}.wav" \
                -filter_complex "[0:a]${norm}[a];[1:a]${norm}[b];[a][b]concat=n=2:v=0:a=1" \
                -c:a pcm_s16le "${out}"
            echo "Wrote ${out}"
        done
    done
    for set in blind:sweep-wot blind-overrun:overrun blind-limiter:limiter; do
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

# Build out/listen-overnight/<profile>.wav for the four tuned profiles: the
# sweep-wot render of the snarl-render binary at the path before, the tier 1
# render of the current tree, and the engine-sim reference, in that order with
# 1 s of silence between them. A plain gain takes each part from its measured
# integrated loudness to -16 LUFS. Writes a spectrogram <profile>.png beside
# each file, and README.txt with the start time and the measured loudness of
# each part.
listen-overnight before: (_listen-overnight before "flat12") (_listen-overnight before "inline4") (_listen-overnight before "flat6") (_listen-overnight before "v8")
    cat out/listen-overnight/flat12.line out/listen-overnight/inline4.line out/listen-overnight/flat6.line out/listen-overnight/v8.line > out/listen-overnight/README.txt
    rm out/listen-overnight/flat12.line out/listen-overnight/inline4.line out/listen-overnight/flat6.line out/listen-overnight/v8.line
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
    scn="scenarios/{{ profile }}/sweep-wot.scn"
    ref="out/reference/engine-sim/{{ profile }}/sweep-wot.wav"
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
    printf '{{ profile }}.wav: before at 0.000 s (%s LUFS), after at %d.%03d s (%s LUFS), engine-sim at %d.%03d s (%s LUFS)\n' \
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
