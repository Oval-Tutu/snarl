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
    go run ./cmd/snarl-harness {{ args }}

# Render a profile offline to a WAV file from an RPM and throttle script.
render profile script *args: (_command "snarl-render")
    go run ./cmd/snarl-render {{ args }} "{{ profile }}" "{{ script }}"

# Compare two renders and write metrics and spectrograms.
compare a b *args: (_command "snarl-compare")
    go run ./cmd/snarl-compare {{ args }} "{{ a }}" "{{ b }}"

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
ref-set profile="":
    #!/usr/bin/env bash
    set -euo pipefail
    r="$(nix build .#engine-sim-render --print-out-paths --no-link)"
    declare -A scripts=(
        [flat12]="${PWD}/reference/engine-sim/engines/flat12.mr"
        [inline4]="${r}/share/engine-sim/engines/atg-video-1/05_honda_vtec.mr"
        [flat6]="${PWD}/reference/engine-sim/engines/flat6.mr"
        [v8]="${PWD}/reference/engine-sim/engines/v8.mr"
    )
    profiles=({{ if profile == "" { "flat12 inline4 flat6 v8" } else { profile } }})
    for p in "${profiles[@]}"; do
        if [[ -z "${scripts[${p}]:-}" ]]; then
            echo "Error: unknown profile '${p}'. Use flat12, inline4, flat6 or v8." >&2
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

# Stop with an error when the command in cmd/<name> does not exist yet.
_command name:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! -d "cmd/{{ name }}" ]]; then
        echo "Error: cmd/{{ name }} does not exist yet, so this recipe has nothing to run." >&2
        exit 1
    fi
