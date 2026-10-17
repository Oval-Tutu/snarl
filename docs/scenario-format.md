# Scenario format

A scenario drives an engine through a timeline of RPM and throttle. The same scenario file drives the engine-sim reference renderer and `snarl-render`, so that the two renders are comparable. `snarl-compare` reads it too, to know the RPM at each moment.

Scenario files live in `scenarios/<profile>/<name>.scn`. The profile directory names match the snarl profile names: `flat12`, `inline4`, `flat6`, `v8`.

## Syntax

A scenario is plain text with one segment on each line. The segments run in order. A `#` starts a comment to the end of the line. Blank lines are ignored. Fields are separated by spaces.

| Segment | Meaning |
| --- | --- |
| `hold RPM THROTTLE SECONDS` | Hold the engine at `RPM` with the throttle at `THROTTLE`. |
| `sweep RPM0 RPM1 THROTTLE SECONDS` | Move the RPM linearly from `RPM0` to `RPM1`, with the throttle at `THROTTLE`. |

- `RPM` is a positive number.
- `THROTTLE` is a number from 0 to 1.
- `SECONDS` is a positive number.

## Meaning in each tool

- The engine-sim renderer turns each line into its `--hold` or `--sweep` flag. Its dynamometer holds the RPM.
- `snarl-render` sets `Snapshot.RPM` and `Snapshot.Throttle` from the segment. It sets `Snapshot.Load` equal to the throttle, because a dynamometer hold has no acceleration. It sets `FlagOverrun` when the throttle is below 0.05, because the dynamometer then drives the engine.
- `snarl-compare` uses the RPM at each moment for the engine order analysis.

## Example

```text
# Full-throttle sweep, then overrun at the top.
sweep 1500 6800 1 8
hold 6800 0 2
```
