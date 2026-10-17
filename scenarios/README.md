# Scenarios

Each profile directory holds the same five scenarios in the shared format (`docs/scenario-format.md`). `just ref-set [profile]` renders them with engine-sim into `out/reference/engine-sim/<profile>/<scenario>.wav`, with a CSV beside each WAV.

| Scenario | Content | Length |
| --- | --- | --- |
| `holds-t000.scn` | 3 s holds at idle and at steps of about 1000 rpm up to the top rpm, throttle 0. A 0.5 s sweep joins each pair of holds. | 20.5 to 31 s |
| `holds-t050.scn` | The same holds at throttle 0.5. | 20.5 to 31 s |
| `holds-t100.scn` | The same holds at throttle 1. | 20.5 to 31 s |
| `sweep-wot.scn` | Full-throttle sweep from idle to the top rpm over 8 s. | 8 s |
| `overrun.scn` | 2 s at the top rpm and throttle 1, then 2 s at throttle 0, then a 6 s sweep down to idle at throttle 0. | 10 s |

Hold steps per profile:

| Profile | Idle | Holds (rpm) | Top rpm | Reason for the top rpm |
| --- | --- | --- | --- | --- |
| `flat12` | 1000 | 1000, 2000, 3000, 4000, 5000, 6000, 6800 | 6800 | Redline. Script rev limit 7000 |
| `inline4` | 800 | 800, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 8400 | 8400 | Redline. Fuel cut 8500 |
| `flat6` | 800 | 800, 2000, 3000, 4000, 5000, 6000, 6600 | 6600 | Below the 6700 governor, which the script uses as its rev limit |
| `v8` | 675 | 675, 2000, 3000, 4000, 5000, 6000 | 6000 | Redline. Fuel cut 6250 |

The top rpm stays below each rev limit, so the limiter does not cut the holds.

In a hold file, the hold at index `i` (0 for idle) starts at `i * 3.5` seconds.
