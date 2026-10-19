#!/usr/bin/env bash
# Usage: engine-sim-main.sh SCRIPT DIR
# Prints the engine-sim script to render for SCRIPT. A script with a main
# node is used as it is. A library script has no main node, so this writes
# a wrapper into DIR that sets its engine node, with a default vehicle and
# a one-gear transmission, and prints the wrapper path.
set -euo pipefail
script="$1"
dir="$2"
if grep -q '^public node main' "${script}"; then
    echo "${script}"
    exit 0
fi
node="$(grep -B1 'alias output __out: *engine;' "${script}" | sed -n 's/^public node \([A-Za-z0-9_]*\).*/\1/p' | tail -n 1)"
if [[ -z "${node}" ]]; then
    echo "Error: ${script} has no main node and no engine node." >&2
    exit 1
fi
mkdir -p "${dir}"
wrapper="$(mktemp "${dir}/${node}.XXXXXX.mr")"
printf 'import "engine_sim.mr"\nimport "%s"\n\npublic node main {\n    set_engine(%s())\n    set_vehicle(vehicle())\n    set_transmission(transmission().add_gear(1.0))\n}\n' \
    "${script}" "${node}" > "${wrapper}"
echo "${wrapper}"
