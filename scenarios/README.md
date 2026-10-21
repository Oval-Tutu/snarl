# Scenarios

The scenarios are in the shared format (`docs/scenario-format.md`):

- `upstream/<profile>/` holds the scenarios of the six embedded profiles (`profiles/README.md`). `tools/fit --upstream-scenarios` writes those of the five engine-sim engines from the engine dump, and `just ref-upstream` renders them with engine-sim into `out/reference/upstream/<profile>/`. The `f6-aircooled` scenarios match the EngineLab scenarios of `just ref-enginelab`. `upstream/README.md` describes them.
- `catalogue/<id>/` holds the scenarios of the profile catalogue, which `just import-profiles` writes (`profiles/catalogue/README.md`). `just ref-set` renders those of the engine-sim engines into `out/reference/engine-sim/catalogue/<id>/`.

| Scenario | Content |
| --- | --- |
| `holds-t000.scn` | 3 s holds at idle and at steps of about 1000 rpm up to the top rpm, throttle 0. A 0.5 s sweep joins each pair of holds. |
| `holds-t050.scn` | The same holds at throttle 0.5. |
| `holds-t100.scn` | The same holds at throttle 1. |
| `sweep-wot.scn` | Full-throttle sweep from idle to the top rpm over 8 s. |
| `overrun.scn` | Full throttle at the top rpm, then closed throttle held, then a closed-throttle sweep down to idle. |

The top rpm is the redline, or 100 rpm below the limiter when the limiter cuts at or below the redline, so the limiter does not cut the holds. In a hold file, the hold at index `i` (0 for idle) starts at `i * 3.5` seconds.
