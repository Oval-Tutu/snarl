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

# Render the upstream scenarios of the embedded profiles with the snarl
# engine and score each against its engine-sim reference in
# out/reference/upstream with the alignment scorecard. The renders go to
# out/score. --dyno --no-pops turns off the idle wander and the own pops,
# which engine-sim does not have.
scorecard *args:
    go run ./tools/scorecard {{ args }}

# The directory of the short exterior crackle clips of the muffler crackle
# pack, crackle_1.wav to crackle_<crackle_count>.wav, for the crackle
# recipes.
crackle_clips := "out/pack/muffler/MufflerCrackleSounds/Unity Engine/Muffler Sound Pack/Assets/Audio/Muffler/Short_Cracles"
crackle_count := "30"

# Measure the 30 short exterior crackle clips of the muffler crackle pack
# with tools/crackle (docs/research/crackle-measure.md). The pack licence
# keeps the clips and the measures of each clip out of the repository, so
# both stay in the ignored out directory. Writes to out: summary.md (the
# clips as one class), summary-classes.md (the k-means classes), and
# clips.csv, shocks.csv and bands.csv.
crackle-measure dir=crackle_clips out="out/crackle":
    #!/usr/bin/env bash
    set -euo pipefail
    files=()
    for i in $(seq 1 {{ crackle_count }}); do
        files+=("{{ dir }}/crackle_${i}.wav")
    done
    mkdir -p "{{ out }}"
    go run ./tools/crackle --k 1 --out "{{ out }}" "${files[@]}" > "{{ out }}/summary.md"
    go run ./tools/crackle "${files[@]}" > "{{ out }}/summary-classes.md"
    echo "Wrote the crackle measures to {{ out }}"

# Fit the crackle model (internal/crackle) to the 30 short exterior crackle
# clips with tools/crackle --fit (docs/research/crackle-model.md). Writes
# the class-level parameters to internal/crackle/crackle.toml and their
# sources to internal/crackle/crackle.sources.toml, and the check of the
# fit trains to out/crackle/fit.md.
crackle-fit dir=crackle_clips:
    #!/usr/bin/env bash
    set -euo pipefail
    files=()
    for i in $(seq 1 {{ crackle_count }}); do
        files+=("{{ dir }}/crackle_${i}.wav")
    done
    mkdir -p out/crackle
    go run ./tools/crackle --fit internal/crackle/crackle.toml "${files[@]}" > out/crackle/fit.md
    echo "Wrote internal/crackle/crackle.toml and the check of the fit to out/crackle/fit.md"

# Render model trains of the crackle model with the committed parameters
# to out/crackle/model, with seeds other than those of the fit, and check
# them against the 30 clips with the same measures. Writes compare.md (the
# check), and summary.md and the CSV files of the trains.
crackle-model trains="60" dir=crackle_clips out="out/crackle/model":
    #!/usr/bin/env bash
    set -euo pipefail
    files=()
    for i in $(seq 1 {{ crackle_count }}); do
        files+=("{{ dir }}/crackle_${i}.wav")
    done
    mkdir -p "{{ out }}"
    go run ./tools/crackle --model {{ trains }} --render "{{ out }}" "${files[@]}" > "{{ out }}/compare.md"
    model=()
    for i in $(seq 1 {{ trains }}); do
        model+=("{{ out }}/crackle_${i}.wav")
    done
    go run ./tools/crackle --k 1 --out "{{ out }}" "${model[@]}" > "{{ out }}/summary.md"
    echo "Wrote the model trains and their check to {{ out }}"

# Measure the own pops of each snarl pop kind with tools/crackle, as the
# baseline for just crackle-measure. snarl-render renders the lift-offs of
# tools/crackle/lift.scn at 44.1 kHz with one kind in use, and once with
# no own pops. tools/crackle subtracts the render with no pops, so that
# the pops alone remain, and cuts it into the runs of pops at quiet spans
# of 0.3 s. Writes out/crackle/baseline/<kind>/ with summary.md.
crackle-baseline profile="v8-ls": (_command "snarl-render")
    #!/usr/bin/env bash
    set -euo pipefail
    out=out/crackle/baseline
    mkdir -p "${out}"
    common=(--rate 44100 --format f32 --set 'pops.character="race"' --set pops.density=1)
    go run ./cmd/snarl-render "${common[@]}" --set pops.density=0 --out "${out}/none.wav" "{{ profile }}" tools/crackle/lift.scn
    for kind in pop boom crackle burble; do
        sets=()
        for k in pop boom crackle burble; do
            w=0
            [[ "${k}" == "${kind}" ]] && w=1
            sets+=(--set "pops.weight_${k}=${w}")
        done
        go run ./cmd/snarl-render "${common[@]}" "${sets[@]}" --out "${out}/${kind}.wav" "{{ profile }}" tools/crackle/lift.scn
        mkdir -p "${out}/${kind}"
        go run ./tools/crackle --k 1 --minus "${out}/none.wav" --split 0.3 --out "${out}/${kind}" "${out}/${kind}.wav" > "${out}/${kind}/summary.md"
    done
    echo "Wrote the pop baseline to ${out}"

# Make the listening files of the crackle pops in out: clips-then-model.wav
# (3 clips, then 3 model trains, at one RMS level, with README.txt), a
# blind test of 20 trials of a clip and a model train in blind/ (key in
# blind/.key), and lift-offs of tools/crackle/lift.scn (or of
# tools/crackle/lift-<engine>.scn when it exists) with the race pops
# at 48 kHz and -19 LUFS: <engine>-liftoffs.wav for each engine, and
# v8-ls-old-then-new.wav with the pops of the old commit first. The old
# render runs in a temporary git worktree of old under out/crackle.
# Writes the length, the integrated loudness and the sample peak of each
# file to levels.txt.
listen-pops-model old="51f94ef" dir=crackle_clips out="out/listen-pops-model": (_command "snarl-render")
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    files=()
    for i in $(seq 1 {{ crackle_count }}); do
        files+=("{{ dir }}/crackle_${i}.wav")
    done
    mkdir -p "{{ out }}"
    go run ./tools/crackle --listen "{{ out }}" "${files[@]}"
    renders="${PWD}/out/crackle/listen-renders"
    mkdir -p "${renders}"
    common=(--rate 48000 --format f32 --set 'pops.character="race"')
    for engine in v8-ls i4-vtec v12-412; do
        lift=tools/crackle/lift.scn
        [[ -f "tools/crackle/lift-${engine}.scn" ]] && lift="tools/crackle/lift-${engine}.scn"
        go run ./cmd/snarl-render "${common[@]}" --out "${renders}/${engine}-new.wav" "${engine}" "${lift}"
        go run ./tools/crackle --join "{{ out }}/${engine}-liftoffs.wav" --lufs -19 "${renders}/${engine}-new.wav"
    done
    wt="out/crackle/worktree-old"
    git worktree add --detach "${wt}" "{{ old }}"
    trap 'git worktree remove --force "${wt}"' EXIT
    scn="${PWD}/tools/crackle/lift.scn"
    (cd "${wt}" && go run ./cmd/snarl-render "${common[@]}" --out "${renders}/v8-ls-old.wav" v8-ls "${scn}")
    git worktree remove "${wt}"
    trap - EXIT
    go run ./tools/crackle --join "{{ out }}/v8-ls-old-then-new.wav" --lufs -19 "${renders}/v8-ls-old.wav" "${renders}/v8-ls-new.wav"
    : > "{{ out }}/levels.txt"
    for f in "{{ out }}"/*.wav "{{ out }}"/blind/*.wav; do
        len=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "${f}")
        lv=$(ffmpeg -nostdin -hide_banner -nostats -i "${f}" -af ebur128=peak=sample -f null - 2>&1 | grep -E '^ +(I|Peak):' | tr -s ' ' | tr '\n' ' ')
        echo "${f#{{ out }}/}: ${len} s,${lv}" >> "{{ out }}/levels.txt"
    done
    cat "{{ out }}/levels.txt"
    echo "Wrote the listening files to {{ out }}"

# The engines and lift-off scenarios of just pop-level and just
# listen-pop-level: tools/crackle/lift-<engine>.scn when it exists, else
# tools/crackle/lift.scn.
pop_level_cases := "v8-ls=tools/crackle/lift.scn f4-ej25=tools/crackle/lift.scn v12-merlin=tools/crackle/lift.scn i4-vtec=tools/crackle/lift-i4-vtec.scn v12-412=tools/crackle/lift-v12-412.scn"

# Measure the loudness and the RMS level of each pop kind over the engine,
# and the cuts of the pop limiter, on the lift-offs of the five engines
# with the race pops (docs/research/pop-level.md). Writes out/pop-level.md.
pop-level:
    mkdir -p out
    go run ./tools/crackle --pop-level {{ pop_level_cases }} > out/pop-level.md
    cat out/pop-level.md

# Make the listening file of the pop level in out: pop-level-variants.wav
# with README.txt. It plays four variants of the pop loudness over the
# engine, -2, -6, -10 and -14 LU against the level before the loudness
# calibration, and each variant plays two lift-offs of each of the five
# engines with the race pops at 48 kHz. Each engine has the gain that puts
# it with no pops at -19 LUFS in every variant. Writes the length, the
# integrated loudness and the sample peak of the file to levels.txt.
listen-pop-level out="out/listen-pop-level":
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    go run ./tools/crackle --pop-variants "{{ out }}" {{ pop_level_cases }}
    f="{{ out }}/pop-level-variants.wav"
    len=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "${f}")
    lv=$(ffmpeg -nostdin -hide_banner -nostats -i "${f}" -af ebur128=peak=sample -f null - 2>&1 | grep -E '^ +(I|Peak):' | tr -s ' ' | tr '\n' ' ')
    echo "pop-level-variants.wav: ${len} s,${lv}" > "{{ out }}/levels.txt"
    cat "{{ out }}/README.txt" "{{ out }}/levels.txt"
    echo "Wrote the listening file to {{ out }}"

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

# Render the full-range references of the five upstream engine-sim engines
# (docs/research/fullrange-scorecard.md), for one engine or for all when
# none is given. tools/scorecard --write-fullrange writes
# scenarios/upstream/<engine>/fullrange-t000.scn to fullrange-t100.scn:
# holds at each throttle row of the cycle table grid every 250 rpm from
# idle to below the limiter, and fullrange-mid-t100.scn: full-throttle
# holds halfway between those RPM. The renderer runs each file twice with the
# settings of just ref-upstream, at the 3 s pre-roll into
# out/reference/upstream/<engine>/ and at a 3.5 s pre-roll into
# out/reference/upstream/calib/<engine>/, the second engine-sim render of
# the calibration (tools/scorecard --calib). The calibration also renders
# sweep-wot and overrun, whose references just ref-upstream renders. The
# renders run in parallel, at most one per CPU.
ref-fullrange engine="":
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
            esac
            echo "Error: $1: bad segment '${line}'" >&2
            exit 1
        done < "$1"
    }
    engines=({{ if engine == "" { "i4-vtec v8-ls f4-ej25 v12-412 v12-merlin" } else { engine } }})
    start=${SECONDS}
    jobs=0
    for e in "${engines[@]}"; do
        if [[ -z "${scripts[${e}]:-}" ]]; then
            echo "Error: unknown engine '${e}'. Use i4-vtec, v8-ls, f4-ej25, v12-412 or v12-merlin." >&2
            exit 1
        fi
        script="$(tools/import/engine-sim-main.sh "${r}/share/engine-sim/engines/${scripts[${e}]}" "${tmp}")"
        go run ./tools/scorecard --write-fullrange --engines "${e}"
        out="out/reference/upstream/${e}"
        calib="out/reference/upstream/calib/${e}"
        mkdir -p "${out}" "${calib}"
        for f in scenarios/upstream/"${e}"/{fullrange-t000,fullrange-t025,fullrange-t050,fullrange-t075,fullrange-t100,fullrange-mid-t100,sweep-wot,overrun}.scn; do
            name="$(basename "${f}" .scn)"
            parse "${f}"
            runs=("${out}:3" "${calib}:3.5")
            # just ref-upstream renders the reference of these two.
            [[ "${name}" == sweep-wot || "${name}" == overrun ]] && runs=("${calib}:3.5")
            for run in "${runs[@]}"; do
                dir="${run%:*}"
                if (( jobs >= $(nproc) )); then
                    wait -n
                    jobs=$((jobs - 1))
                fi
                "${r}/bin/engine-sim-render" --script "${script}" "${segments[@]}" --volume 0.5 --settle "${run##*:}" \
                    --out "${dir}/${name}.wav" --csv "${dir}/${name}.csv" --csv-every 100 > "${dir}/${name}.log" 2>&1 &
                jobs=$((jobs + 1))
            done
        done
    done
    while (( jobs > 0 )); do
        wait -n
        jobs=$((jobs - 1))
    done
    echo "Rendered the full-range references of ${engines[*]} in $((SECONDS - start)) s"

# Render the drive references of the five upstream engine-sim engines
# (docs/research/drive-scorecard.md), for one engine or for all when none is
# given. snarl-harness -scenario writes scenarios/upstream/<engine>/drive.scn
# from the scripted drive of the harness: one sweep per tick of 60 per
# second. The renderer runs it as just ref-fullrange does, at the 3 s
# pre-roll into out/reference/upstream/<engine>/ and at a 3.5 s pre-roll
# into out/reference/upstream/calib/<engine>/, in parallel.
ref-drive engine="": (_command "snarl-harness")
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
    engines=({{ if engine == "" { "i4-vtec v8-ls f4-ej25 v12-412 v12-merlin" } else { engine } }})
    start=${SECONDS}
    (cd cmd/snarl-harness && go build -o "${tmp}/snarl-harness" .)
    pids=()
    for e in "${engines[@]}"; do
        if [[ -z "${scripts[${e}]:-}" ]]; then
            echo "Error: unknown engine '${e}'. Use i4-vtec, v8-ls, f4-ej25, v12-412 or v12-merlin." >&2
            exit 1
        fi
        script="$(tools/import/engine-sim-main.sh "${r}/share/engine-sim/engines/${scripts[${e}]}" "${tmp}")"
        f="scenarios/upstream/${e}/drive.scn"
        "${tmp}/snarl-harness" -profile "${e}" -scenario "${f}"
        segments=()
        while IFS= read -r line; do
            read -r kind a b c d extra <<< "${line%%#*}" || true
            case "${kind:-}" in
                "") continue ;;
                hold) [[ -n "${c}" && -z "${d}" ]] && segments+=(--hold "${a}:${b}:${c}") && continue ;;
                sweep) [[ -n "${d}" && -z "${extra}" ]] && segments+=(--sweep "${a}:${b}:${c}:${d}") && continue ;;
            esac
            echo "Error: ${f}: bad segment '${line}'" >&2
            exit 1
        done < "${f}"
        for run in "out/reference/upstream/${e}:3" "out/reference/upstream/calib/${e}:3.5"; do
            dir="${run%:*}"
            mkdir -p "${dir}"
            "${r}/bin/engine-sim-render" --script "${script}" "${segments[@]}" --volume 0.5 --settle "${run##*:}" \
                --out "${dir}/drive.wav" --csv "${dir}/drive.csv" --csv-every 100 > "${dir}/drive.log" 2>&1 &
            pids+=($!)
        done
    done
    for p in "${pids[@]}"; do
        wait "${p}"
    done
    echo "Rendered the drive references of ${engines[*]} in $((SECONDS - start)) s"

# Render the listening references of the five upstream engine-sim engines
# for just listen-ab, for one engine or for all when none is given.
# tools/scorecard --write-listen writes
# scenarios/upstream/<engine>/listen-hold-70.scn and listen-hold-90.scn:
# a full-throttle hold at the full-range RPM closest to 0.7 and 0.9 of the
# redline, 0.5 s of settling and then 3 s. The renderer runs each
# listen-hold-*.scn file of the engine, these two and any other, as just
# ref-fullrange does, at the 3 s pre-roll into
# out/reference/upstream/<engine>/ and at a 3.5 s pre-roll into
# out/reference/upstream/calib/<engine>/, in parallel.
ref-listen engine="":
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
    engines=({{ if engine == "" { "i4-vtec v8-ls f4-ej25 v12-412 v12-merlin" } else { engine } }})
    start=${SECONDS}
    pids=()
    for e in "${engines[@]}"; do
        if [[ -z "${scripts[${e}]:-}" ]]; then
            echo "Error: unknown engine '${e}'. Use i4-vtec, v8-ls, f4-ej25, v12-412 or v12-merlin." >&2
            exit 1
        fi
        script="$(tools/import/engine-sim-main.sh "${r}/share/engine-sim/engines/${scripts[${e}]}" "${tmp}")"
        go run ./tools/scorecard --write-listen --engines "${e}"
        for f in scenarios/upstream/"${e}"/listen-hold-*.scn; do
            name="$(basename "${f}" .scn)"
            segments=()
            while IFS= read -r line; do
                read -r kind a b c d extra <<< "${line%%#*}" || true
                case "${kind:-}" in
                    "") continue ;;
                    hold) [[ -n "${c}" && -z "${d}" ]] && segments+=(--hold "${a}:${b}:${c}") && continue ;;
                    sweep) [[ -n "${d}" && -z "${extra}" ]] && segments+=(--sweep "${a}:${b}:${c}:${d}") && continue ;;
                esac
                echo "Error: ${f}: bad segment '${line}'" >&2
                exit 1
            done < "${f}"
            for run in "out/reference/upstream/${e}:3" "out/reference/upstream/calib/${e}:3.5"; do
                dir="${run%:*}"
                mkdir -p "${dir}"
                "${r}/bin/engine-sim-render" --script "${script}" "${segments[@]}" --volume 0.5 --settle "${run##*:}" \
                    --out "${dir}/${name}.wav" --csv "${dir}/${name}.csv" --csv-every 100 > "${dir}/${name}.log" 2>&1 &
                pids+=($!)
            done
        done
    done
    for p in "${pids[@]}"; do
        wait "${p}"
    done
    echo "Rendered the listening references of ${engines[*]} in $((SECONDS - start)) s"

# Make A/B listening files of engine-sim against snarl in out/listen-ab for
# the five upstream engines, from the references of just ref-listen and just
# ref-drive, which it renders first when they are missing. For each engine,
# out/listen-ab/<engine>/ has hold-70.wav and hold-90.wav (the listening
# holds of just ref-listen, 3 s after the 0.5 s of settling) and drive.wav
# (the whole scripted drive of drive.scn). Each file plays engine-sim, 1 s
# of silence, then snarl from the same scenario time. tools/scorecard
# renders snarl with --dyno --no-pops, as the scorecards do, and writes the
# measures of each pair. A plain gain gives both parts the same integrated
# loudness, each part has a 10 ms fade in and an 80 ms fade out, and one
# more gain takes the file to -19 LUFS. The recipe stops when a file has a
# true peak at or above -1 dBTP. Beside each WAV file are a
# spectrogram PNG of the two parts, one above the other with the same
# axes and colour scale, and a TXT file with the measures. README.txt lists
# the files. The snarl renders and the measures go to out/score/listen-ab.
listen-ab:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    engines=(i4-vtec v8-ls f4-ej25 v12-412 v12-merlin)
    ref=out/reference/upstream
    scn=scenarios/upstream
    work=out/score/listen-ab
    out=out/listen-ab
    target=-19
    for e in "${engines[@]}"; do
        if [[ ! -f "${scn}/${e}/listen-hold-70.scn" || ! -f "${scn}/${e}/listen-hold-90.scn" \
            || ! -f "${ref}/${e}/listen-hold-70.wav" || ! -f "${ref}/${e}/listen-hold-90.wav" ]]; then
            "{{ just_executable() }}" ref-listen "${e}"
        fi
        if [[ ! -f "${scn}/${e}/drive.scn" || ! -f "${ref}/${e}/drive.wav" ]]; then
            "{{ just_executable() }}" ref-drive "${e}"
        fi
    done
    rm -rf "${work}" "${out}"
    mkdir -p "${work}" "${out}"
    go run ./tools/scorecard --dyno --no-pops --engines "$(IFS=,; echo "${engines[*]}")" \
        --scenarios listen-hold-70,listen-hold-90,drive --out "${work}" --summary "${work}" > "${work}/score.md"
    ff=(ffmpeg -nostdin -hide_banner -loglevel error -y)
    # Prints the integrated loudness in LUFS of file $1 from $2 to $3 s.
    lufs() {
        local l
        l="$(ffmpeg -nostdin -hide_banner -i "$1" -af "atrim=start=$2:end=$3,ebur128=framelog=quiet" -f null - 2>&1 | grep -E '^ +I:')"
        l="${l#*I:}"
        l="${l%LUFS}"
        echo "${l// /}"
    }
    # Prints the duration in seconds of file $1.
    duration() {
        ffprobe -v error -show_entries format=duration -of csv=p=0 "$1"
    }
    fmt="aformat=sample_fmts=flt:sample_rates=48000:channel_layouts=mono"
    # Fade each part in over 10 ms and out over 80 ms, so that the joins do
    # not click.
    fade="afade=t=in:d=0.01,areverse,afade=t=in:d=0.08,areverse"
    spec="showspectrumpic=s=1600x480:mode=combined:color=intensity:scale=log:fscale=lin:drange=120:legend=1"
    label="drawtext=fontcolor=white:fontsize=24:box=1:boxcolor=black@0.6:x=w-tw-20:y=8:text"
    {
        echo "A/B listening files of engine-sim against snarl, made by just listen-ab."
        echo
        echo "Each file plays engine-sim, 1 s of silence, then snarl, cut from the same scenario time."
        echo "The two parts have the same integrated loudness, each has a 10 ms fade in and an 80 ms fade out,"
        echo "and each file is at ${target} LUFS with a true peak below -1 dBTP."
        echo "snarl renders at 48 kHz, tier 1, with the dynamometer settings and no pops (tools/scorecard --dyno --no-pops)."
        echo "engine-sim renders with the 3 s pre-roll of the references. Throttle 1 is full throttle."
        echo "Beside each WAV file: a PNG spectrogram (engine-sim above, snarl below) and a TXT file with the measures."
        echo
    } > "${out}/README.txt"
    for e in "${engines[@]}"; do
        mkdir -p "${out}/${e}"
        redline="$(grep -m 1 '^redline_rpm = ' "profiles/${e}.toml")"
        redline="${redline##* }"
        for name in hold-70 hold-90 drive; do
            s="${name}"
            [[ "${name}" == hold-* ]] && s="listen-${name}"
            a="${ref}/${e}/${s}.wav"
            b="${work}/${e}/${s}.wav"
            if [[ "${name}" == drive ]]; then
                t0=0
                t1="$(printf '%s\n%s\n' "$(duration "${a}")" "$(duration "${b}")" | sort -g | head -n 1)"
            else
                read -r _ rpm _ t1 < <(grep -v '^#' "${scn}/${e}/${s}.scn")
                t0=0.5
            fi
            la="$(lufs "${a}" "${t0}" "${t1}")"
            lb="$(lufs "${b}" "${t0}" "${t1}")"
            cut="atrim=start=${t0}:end=${t1},asetpts=PTS-STARTPTS,${fmt}"
            "${ff[@]}" -i "${a}" -i "${b}" -filter_complex \
                "[0:a]${cut},volume='pow(10,(${target}-(${la}))/20)',${fade}[a];[1:a]${cut},volume='pow(10,(${target}-(${lb}))/20)',${fade}[b];aevalsrc=0:s=48000:d=1,${fmt}[s];[a][s][b]concat=n=3:v=0:a=1" \
                -c:a pcm_f32le "${work}/${e}/${name}-ab.wav"
            # The silence and the fades take the file a little below the
            # parts, so one gain on the whole file takes it to the target.
            l="$(lufs "${work}/${e}/${name}-ab.wav" 0 "$(duration "${work}/${e}/${name}-ab.wav")")"
            "${ff[@]}" -i "${work}/${e}/${name}-ab.wav" -af "volume='pow(10,(${target}-(${l}))/20)'" -c:a pcm_f32le "${out}/${e}/${name}.wav"
            t0="$(awk -v t="${t0}" 'BEGIN { printf "%.3f", t }')"
            t1="$(awk -v t="${t1}" 'BEGIN { printf "%.3f", t }')"
            len="$(awk -v a="${t0}" -v b="${t1}" 'BEGIN { printf "%.3f", b - a }')"
            peak="$(ffmpeg -nostdin -hide_banner -i "${out}/${e}/${name}.wav" -af ebur128=peak=true:framelog=quiet -f null - 2>&1 | grep -A 1 'True peak:' | grep -E '^ +Peak:')"
            peak="${peak#*Peak:}"
            peak="${peak%dBFS}"
            peak="${peak// /}"
            if ! awk -v p="${peak}" 'BEGIN { exit !(p < -1) }'; then
                echo "Error: ${out}/${e}/${name}.wav has a true peak of ${peak} dBTP, at or above -1 dBTP." >&2
                exit 1
            fi
            sb="$(awk -v l="${len}" 'BEGIN { printf "%.3f", l + 1 }')"
            "${ff[@]}" -i "${out}/${e}/${name}.wav" -filter_complex \
                "[0:a]asplit[x][y];[x]atrim=end=${len},${spec},${label}='engine-sim  ${e} ${name}'[a];[y]atrim=start=${sb},asetpts=PTS-STARTPTS,${spec},${label}='snarl  ${e} ${name}'[b];[a][b]vstack" \
                -frames:v 1 "${out}/${e}/${name}.png"
            {
                cat "${work}/${e}/${s}.txt"
                echo
                echo "Integrated loudness of the parts before the gain: engine-sim ${la} LUFS, snarl ${lb} LUFS."
                echo "The parts play from ${t0} s to ${t1} s of the scenario. The file has a true peak of ${peak} dBTP."
            } > "${out}/${e}/${name}.txt"
            if [[ "${name}" == drive ]]; then
                what="the scripted drive of ${scn}/${e}/drive.scn (idle, full-throttle pulls, lifts, coasts and part throttle; throttle 0 to 1 as scripted)"
            else
                what="a full-throttle hold at ${rpm} rpm (the full-range grid RPM closest to 0.${name#hold-} of the redline at ${redline} rpm), throttle 1"
            fi
            printf '%s/%s.wav: %s. engine-sim plays from 0.000 to %s s, silence to %s s, snarl from %s to %s s.\n' \
                "${e}" "${name}" "${what}" "${len}" "${sb}" "${sb}" "$(awk -v l="${len}" 'BEGIN { printf "%.3f", 2 * l + 1 }')" >> "${out}/README.txt"
            echo "Wrote ${out}/${e}/${name}.wav"
        done
    done
    echo "Wrote the listening files to ${out}"

# Play the full-range scenarios of just ref-fullrange from the cycle tables
# through the exact synthesiser port with tools/cycles play, for one engine
# or for all when engine is "all", into out/fullrange/<name>/<engine>/, so
# that tools/scorecard --against out/fullrange/<name> scores them against
# engine-sim (docs/research/fullrange-scorecard.md). args go to tools/cycles
# play, for example --no-residual. The engines play in parallel.
fullrange-play name engine="all" *args:
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build -f reference/engine-sim --print-out-paths --no-link)"
    engines=({{ if engine == "all" { "i4-vtec v8-ls f4-ej25 v12-412 v12-merlin" } else { engine } }})
    start=${SECONDS}
    go build -o "out/fullrange/cycles" ./tools/cycles
    pids=()
    for e in "${engines[@]}"; do
        for f in scenarios/upstream/"${e}"/fullrange-*.scn; do
            out/fullrange/cycles play --tables "profiles/cycles/${e}.bin" --es "${r}/share/engine-sim/es" \
                --out-dir "out/fullrange/{{ name }}/${e}" {{ args }} "${f}" > /dev/null &
            pids+=($!)
        done
    done
    for p in "${pids[@]}"; do
        wait "${p}"
    done
    echo "Played the full-range scenarios of ${engines[*]} into out/fullrange/{{ name }} in $((SECONDS - start)) s"

# Build the cycle tables of the five upstream engine-sim engines
# (docs/research/cycle-tables.md), for one engine or for all when none is
# given. tools/cycles scenarios writes the grid scenarios into
# scenarios/upstream/<engine>/grid from the first and last hold of
# holds-t100.scn and the limiter RPM of profiles/<engine>.toml: one file per
# throttle row (0, 0.25, 0.5, 0.75 and 1) with a hold at idle, every
# multiple of the RPM step and the top, the last multiple of the step below
# 0.998 of the limiter RPM or the last hold when that is higher, cut.scn
# with ignition-off holds, step.scn with throttle steps for the intake
# lag, and close.scn with closing steps for the closing transient. The
# renderer runs each file with --dump-synth into
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
        # The grid goes on above the last hold to below the limiter RPM of
        # the profile, as the full-range scenarios do.
        limiter="$(grep -m 1 '^limiter_rpm = ' "profiles/${e}.toml")"
        go run ./tools/cycles scenarios --holds "scenarios/upstream/${e}/holds-t100.scn" --name "${e}" \
            --step "${steps[${e}]}" --limiter "${limiter##* }" --out "${scn}"
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
        go run ./tools/cycles build --out "profiles/cycles/${e}.bin" --step "${out}/step" --close "${out}/close" \
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
# upstream references of the five embedded profiles: each snarl render
# out/snarl/<profile>/<scenario>.wav followed by its reference, and blind
# sets of the snarl sweep-wot and overrun renders in a new random order with
# the answer in key.txt. The references are out/reference/upstream/<profile>
# of just ref-upstream. All parts are loudness-normalised with loudnorm.
listen:
    #!/usr/bin/env bash
    set -euo pipefail
    profiles=(i4-vtec v8-ls f4-ej25 v12-412 v12-merlin)
    scenarios=(sweep-wot overrun holds-t100)
    if ! command -v ffmpeg >/dev/null; then
        echo "Error: ffmpeg is not on PATH. Run this recipe in nix develop." >&2
        exit 1
    fi
    # Prints the reference of profile $1 and scenario $2.
    ref() {
        echo "out/reference/upstream/$1/$2.wav"
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

# Build out/listen-overnight/<profile>.wav for the five embedded profiles:
# the sweep-wot render of the snarl-render binary at the path before, the
# render of the current tree, and the upstream reference, in that order with
# 1 s of silence between them. The binary before must know the five
# profiles. A plain gain takes each part from its measured integrated
# loudness to -16 LUFS. Writes a spectrogram <profile>.png beside each file,
# and README.txt with the start time and the measured loudness of each part.
listen-overnight before: (_listen-overnight before "i4-vtec") (_listen-overnight before "v8-ls") (_listen-overnight before "f4-ej25") (_listen-overnight before "v12-412") (_listen-overnight before "v12-merlin")
    cd out/listen-overnight && cat i4-vtec.line v8-ls.line f4-ej25.line v12-412.line v12-merlin.line > README.txt
    cd out/listen-overnight && rm i4-vtec.line v8-ls.line f4-ej25.line v12-412.line v12-merlin.line
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

# Rebuild the embedded profiles in profiles (profiles/README.md) for the five
# engine-sim engines: i4-vtec, v8-ls, f4-ej25, v12-412 and v12-merlin.
# tools/import --reference reads the cycle tables profiles/cycles/<id>.bin of
# just cycles-grid, the references of just ref-upstream and the scenarios in
# scenarios/upstream, copies the impulse responses that the tables name from
# the pinned engine-sim package into profiles/ir, and writes each profile
# with its sources file, and the README. Give id=DIR to use other reference
# renders for an engine, for example v12-412=out/reference/v12-412-10k.
import-reference *refs:
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build -f reference/engine-sim --print-out-paths --no-link)"
    flags=()
    for ref in {{ refs }}; do
        flags+=(--ref "${ref}")
    done
    go run ./tools/import --reference profiles --scenarios scenarios/upstream --grid out/cycles/grid \
        --engine-sim-share "${r}/share/engine-sim" "${flags[@]}"
