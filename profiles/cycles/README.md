# Cycle tables

Each `<engine>.bin` holds the cycle tables of one engine-sim engine: the synthesiser input of each exhaust channel over one 720-degree engine cycle on a grid of RPM and throttle holds, the slices of the closing transient after a full-throttle close, a model of the part of the input that does not repeat from cycle to cycle, and the intake lag. `just cycles-grid` writes the files from engine-sim renders of the unchanged upstream scripts. Do not edit them by hand: run the recipe again. `docs/research/cycle-tables.md` gives the method, the grid and the scores. `internal/cycles` reads and plays them, and the library embeds them.

| Engine | Points | Closing transient slices | Bytes |
| --- | --- | --- | --- |
| `i4-vtec` | 247 | 69 | 1390459 |
| `v8-ls` | 185 | 51 | 823085 |
| `f4-ej25` | 185 | 51 | 1145861 |
| `v12-412` | 464 | 142 | 1311305 |
| `v12-merlin` | 87 | 24 | 433130 |

## Format

All values are little endian.

1. The magic `SNARLCYC`, the version (uint32, 1) and the length of the JSON header (uint32).
2. The JSON header: the engine and script names, the input rate, the synthesiser audio parameters, the channels (impulse response path relative to the engine-sim `es/` directory, impulse response volume, exhaust delay in steps), the intake lag, and per point its RPM, throttle, ignition state, measured RPM, cycles and harmonic count. A slice of the closing transient also has `since_s`, the time from the close to the middle of the slice. A steady point has none.
3. Per point, in the order of the header: the crank speed profile (64 float16 values), then per channel the average cycle (a float32 scale, then 2H+1 float16 values over the scale: the mean, then the real and imaginary parts of harmonics 1 to H), then per channel the residual envelopes (a float32 scale, then 9 bands times 64 sectors of float16 values over the scale).

The tables hold no impulse response. The impulse responses that the channels name are unchanged copies in `profiles/ir/`, which `just import-reference` writes from the engine-sim package.
