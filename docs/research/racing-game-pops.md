# Exhaust pops in real supercars and racing games

Status: research complete. Date: 2026-10-19. Worker report for the pop retune (the user finds the overrun pops "too much" for supercars and wants fewer, more random pops tied to gear changes and the run down from high revs).

Scope: read-only web research. Sources are graded in each table: official (manufacturer, developer, middleware documentation, patch notes), press (interview or trade article), tuner (an ECU tuning company), or forum (Reddit, PistonHeads, GTPlanet, Forza forums). Forum evidence gives player reception and anecdotes only. Items marked unverified come from memory or from a source that this report could not fetch. One fact per source where the text allows.

snarl today (from `internal/model/pops.go` and `docs/scoping.md`): four pop kinds (pop, boom, crackle, burble) picked by profile weights, triggered on overrun only. The model arms when the throttle opens above 0.20 at or above `min_rpm`, fires when the throttle closes above 1.65 times idle RPM, spaces events 110 to 264 ms apart inside a run and 300 to 700 ms between runs, and halves the run probability over 2.5 s of overrun. `ShiftUp`, `ShiftDown` and `Limiter` counters and the `Overrun` and `FinalGear` flags are in the snapshot.

## 1. Answer

Factory supercars pop in three places: a bang on a fast, full-throttle upshift (ignition cut or heavy spark retard during the torque interruption), a short burst of crackle in the first second or two after a fast lift from high RPM, and a pop or two with a downshift blip when the exhaust valves are open. Factory calibrations gate the overrun crackle hard: the engine must be warm, the catalyst below a limit, the RPM and pedal above thresholds, the pedal must drop fast, and a countdown then ends the effect, so a long overrun goes quiet after a few seconds. Aftermarket "pops and bangs" maps lengthen the fuel-cut delay and retard the spark 10 to 30 degrees, which gives the gunfire that owners hear as too much. Emissions rules since 2018 (gasoline particulate filters) muted most factory pops in Europe.

Games follow the same three triggers. Forza Horizon 5 chops backfire samples per car and fires them on lift-off, with an anti-lag upgrade that fires per cylinder when the throttle is below 100 per cent and the turbo slows. Players called that upgrade "constant" and "overkill for supercars". Need for Speed exposes an "Overrun" slider from "subtle" to "raw". Assetto Corsa gates backfires in `sounds.ini` by a maximum throttle, an RPM range and a throttle-drop threshold. Middleware practice is a random pool with weights, "avoid repeating the last x", a minimum spawn interval, a polyphony cap and a do-not-restart flag.

Recommendation for snarl (section 5): keep the four kinds, add three event triggers (upshift, downshift, lift-off from high RPM) with per-event probability, count and cool-down, keep a sparse burble on long overrun, cut the base density by about half, end a lift-off run with a countdown as the factory calibrations do, and avoid repeats with a per-kind shuffle bag. The game needs to send nothing new. A single optional exhaust character value (stock, sport, race) scales density, gain and kind weights for car upgrades.

## 2. Real cars

### 2.1 Mechanisms

| Event | Mechanism | Sound | Source |
| --- | --- | --- | --- |
| Lift-off overrun (factory) | The ECU keeps a little fuel for a few hundred milliseconds after the pedal lifts and retards the spark so that combustion continues as the exhaust valve opens. Off-cycle, near legal air-fuel ratio. | Crackle and pops in the first seconds of overrun | MotorTrend (press): "The production cars you hear burbling and popping on engine overrun do so under off-cycle conditions and most likely at legal air-fuel ratios, just with late ignition that lets some fuel burn in the exhaust pipe." <https://www.motortrend.com/how-to/what-is-anti-lag> |
| Lift-off overrun (factory, Ford Focus RS strategy) | Lean lambda target plus a spark torque ratio, a "lean misfire" that burns in the exhaust. | Pops and crackles | StrikeEngine reprint of a FocusRS.org reverse engineering post (tuner) <https://strikeengine.com/exhaust-burbelspopscrackles-how-its-done/> |
| Fast upshift (dual-clutch, automatic, dog box) | The transmission asks for a torque cut. The fast path is spark cut or heavy retard, because the throttle is too slow. Fuel still burns, in the manifold. | One bang or "snap crackle and pop", a flat "waffle" for 100 ms | PistonHeads (forum, calibration engineer post): on a high load sporty shift done in about 100 ms the torque controller uses "the ignition angle controller" with "extremely retarding the ignition angle, at a constant AFR". <https://www.pistonheads.com/gassing/topic.asp?f=66&h=0&t=1521874> |
| Fast upshift (BMW ZF 8HP and DCT) | Ignition cut on one or more cylinders with injectors open, chosen only for fast, high-RPM, high-load shifts. The mixture burns at the turbo. | Distinct bang | xAutomotive (tuner): "Cutting torque by ignition cut is most desirable on fast, high-RPM, and high-load shifts." Ratio-change phase 100 to 500 ms. <https://www.xautomotive.com/blogs/news/the-fart-secret> |
| Upshift (race dog box) | Ignition cut of 40 to 60 ms unloads the dogs. Unburned fuel pops in the exhaust. | One pop per shift | HP Academy (tuner school) <https://www.hpacademy.com/q-and-a/pops-and-bangs-why-it-occurs-when-changing-gears/> |
| Upshift (well calibrated race car) | A 60 per cent cut followed by 20 to 30 degrees of retard over 50 to 100 ms gives "little to no popping". Popping means the cut is too long. | Near silent | HP Academy forum answer (tuner school) <https://www.hpacademy.com/forum/engine-building-fundamentals/show/gear-shift-with-ignition-cut/> |
| Downshift blip | Automated rev matching opens the throttle to a target RPM, sometimes with a fuel cut near the target. The blip itself is throttle, not misfire. Pops come from the overrun fuel after the blip, and only when the exhaust bypass valves are open. | Blip then one or two pops | Motor1 (press, GM calibrator interview) <https://www.motor1.com/features/707050/how-auto-rev-match-works/>. FerrariChat (forum): on a 599 the bypass valves "slam shut" off throttle "unless you are up around 5-6000 rpm", which stifles the overrun popping. <https://www.ferrarichat.com/forum/threads/throttle-blip.388211/> |
| Anti-lag (rally) | Air bypasses the cylinder and a rich mixture burns before the turbine. "Pretty uncontrolled and very aggressive, sounding like gunfire!" | Gunfire | PistonHeads (forum, same post as above) and MotorTrend (press) |
| Rev limiter | A spark-cut limiter sends unburned fuel to the exhaust. Production cars prefer fuel cut for emissions and catalyst life. | Bangs on the limiter | Engineering Stack Exchange (forum) <https://engineering.stackexchange.com/questions/14831/why-do-fuel-injected-supercars-still-backfire> |
| Cold start | Retarded spark heats the catalyst at start-up. The Ford strategy will not arm until the coolant is warm. | Rare pops, mostly from secondary air systems | StrikeEngine (tuner): "Minimum Engine Coolant temperature reached" is an availability condition. Equilibrium Tuning (tuner): VW group pumps secondary air during warm-up. <https://eqtuning.com/blogs/technical-write-ups/burbles-pops-and-bangs-crackles> |
| Old cars without a mapped effect | Rich mixtures on overrun, no fuel cut, and exhaust leaks give natural pops. Older muscle cars are "less likely to exhibit" the modern backfire. | Occasional burble | Engineering Stack Exchange (forum, two answers) |

### 2.2 Factory gating logic

The Ford Focus RS strategy is the best documented factory calibration. Its state machine is a direct template for snarl:

- Unavailable until: minimum RPM, minimum pedal, minimum coolant temperature, catalyst temperature below a maximum.
- Armed when: RPM and pedal exceed thresholds that depend on the drive mode.
- Active when: the pedal rate exceeds a trigger rate. "You have to lift the throttle fast enough for it to trigger."
- While active: lean lambda, spark torque ratio, and "a configurable countdown before it will become inactive".

Source: StrikeEngine reprint (tuner) <https://strikeengine.com/exhaust-burbelspopscrackles-how-its-done/>.

Two other sources confirm the countdown. A PistonHeads tuner post (forum): "These days this function is usually timed. So after a few seconds the cut off is activated the fuel and the noises disappear." <https://www.pistonheads.com/gassing/topic.asp?f=66&h=0&t=1521874>. A Jaguar F-Type owner (forum): "if you try it for too long on the same gear it will stop and you won't get any crackles until changing to another gear." <https://ftypeforums.co.uk/threads/exhaust-note.3641/>. A South African tuner (tuner): "There is a small window during overrun when the injectors are still supplying a small amount of fuel ... After this window closes, the injectors turn off and the crackles stop." <https://thanas.co.za/blog/25-tuning-for-exhaust-crackles/body>

### 2.3 How often in a typical drive

No source gives a count per minute. The qualitative picture:

| Car and state | Observed frequency | Source |
| --- | --- | --- |
| Jaguar F-Type P300 (2019, GPF) | "I only get a pop or two when letting off the gas", more with repeated throttle taps at 3000 to 5000 RPM, then it stops in the same gear. | F-Type forum (forum) <https://ftypeforums.co.uk/threads/exhaust-note.3641/> |
| Jaguar F-Type R (2021, US, no GPF) | Lifting at about 3500 RPM in first gear "unloads a scary level of pops and crackles". Every upshift past 4000 RPM gives "a chainsaw-like, ripping braaaaap". Quiet-start mode on start-up. | Autoblog (press) <https://www.autoblog.com/2020/08/04/2021-jaguar-f-type-r-exhaust-test/> |
| Jaguar F-Type MY19 (GPF plus software) | Owner "couldn't make it pop and bang but it did snort on gear changes, which the latest models all do". | F-Type forum (forum) <https://ftypeforums.co.uk/threads/petrol-particulate-filter.6208/> |
| Jaguar F-Type V8 2014 to 2016 | A dealer software update reduced the pops. Owners say that Jaguar linked the pops to oxygen sensor and catalyst warranty claims. | PistonHeads (forum) <https://www.pistonheads.com/gassing/topic.asp?f=85&h=0&i=140&t=1890689> |
| BMW G-series (ZF 8HP) | Most G-series cars ship with the ignition-cut shift method "turned off entirely". Stock exhausts with filters make the bang "barely noticeable". BMW plays an artificial bang through the cabin speakers on the G8X M3 and M4. | xAutomotive (tuner) <https://www.xautomotive.com/blogs/news/the-fart-secret> |
| Mercedes-AMG A45 | Stock car has "crazy loud upshift pops and bangs and explosions and downshift pops and burbles". | GTPlanet (forum) <https://www.gtplanet.net/forum/threads/the-gt7-car-sounds-thread.435519/> |
| Europe after Euro 6d (2018 on) | Gasoline particulate filters muffle the exhaust and make artificial pops hard to add. The F-Type R lost its "machine-gun-like exhaust character". | CarThrottle (press, quoting Aston Martin chief engineer Matt Becker) <https://www.carthrottle.com/news/what-are-petrol-particulate-filters-and-why-do-they-make-cars-sound-worse> |
| Europe, WLTP | Cars are now tested in loud mode, and Lamborghini avoided filters in its then-current range. | Autotrader UK (press) <https://www.autotrader.co.uk/content/news/this-is-why-cars-sound-terrible-in-2020> |

Conclusion: a modern factory supercar in sport mode pops on most hard upshifts above about 4000 RPM, gives a one to two second crackle after a fast lift from above about 3000 RPM, and then goes quiet. A long coast is silent. In normal mode or with a particulate filter the car is almost silent. Confidence: medium. The evidence is consistent but anecdotal.

### 2.4 Factory against aftermarket

| Parameter | Factory | Aftermarket "pops and bangs" | Source |
| --- | --- | --- | --- |
| Spark retard on overrun | Spark torque ratio to a target, lean lambda | 10 to 15 degrees moderate, up to 30 degrees aggressive | GOW Tuning (tuner) <https://gowtuning.com/pop-bang/> |
| Fuel | Lean (one PistonHeads tuner aims for 15:1) | Rich, 11.5:1 to 12.5:1 | GOW Tuning (tuner), PistonHeads (forum) |
| Fuel-cut delay | Milliseconds, lengthened a little | Lengthened by the tuner's choice | Equilibrium Tuning (tuner) |
| Trigger RPM | Drive-mode dependent | "often around 3000 RPM" and continues as the revs drop | Eagle Tuning (tuner) <https://blog.eagletuning.com/pop-bang-tuning-faqs/> |
| Warm engine | Required | Required, "above 70-80°C" | Eagle Tuning (tuner) |
| Load gate | Pedal near closed | Light-load deceleration only | GOW Tuning (tuner) |
| Character names | "impulse combustion" (OEM term) | Burble (soft, continuous), crackle, pops and bangs (sharp), flame map (fuel dump) | Thanas (tuner), GOW Tuning (tuner) |

Reception of aftermarket maps is poor even among enthusiasts: "the widely shared opinion, even among car enthusiasts, is that pop and bang noises simply don't sound good" (Sgcarmart, press opinion) <https://www.sgcarmart.com/articles/advice/what-they-dont-tell-you-your-pop-and-bang-map-isnt-anti-lag-and-it-isnt-doing-anything-for-your-cars-performance-31071>. The user's reaction to the current snarl density matches this. The target is the factory sport-mode character, not the tuner map.

### 2.5 Engine type differences

| Factor | Effect on pops | Source |
| --- | --- | --- |
| Turbocharged | Strongest effect. Hot exhaust flow and boost control amplify the sound. Anti-lag and upshift cuts are turbo features. | GOW Tuning (tuner): "Turbocharged petrol engines → strongest effect. Naturally aspirated petrol engines → milder effect." Eagle Tuning (tuner): on NA engines the sound is "less aggressive and more subtle". |
| Naturally aspirated | Spark and fuel only. Milder, rounder. | Same sources |
| Free-flowing exhaust | Louder and sharper pops. Filters, multiple catalysts and mufflers hide them. Tuners dial the effect back on straight pipes "to avoid sounding like a gun range". | xAutomotive (tuner), Eagle Tuning (tuner) |
| Cross-plane V8 | Uneven exhaust pulses give the low-order burble. One synthesiser vendor sets a higher overrun pop probability for its cross-plane preset than for its even-firing V8 (35 against 25 per cent). | Motor Authority (press) <https://www.motorauthority.com/news/1144083_science-why-different-types-of-engines-sound-the-way-they-do>, EV-Engine (vendor blog, synthetic presets, not measurements) <https://ev-engine.com/blog/crossplane-and-flat-plane-v8> |
| Flat-plane V8, flat-12, flat-6 | Even firing. No source ties crank layout to pop frequency. Pop character follows the exhaust (pipe length, silencers) more than the crank. | Car and Driver (press) <https://www.caranddriver.com/features/a15107374/this-is-why-various-engine-types-sound-so-different-feature/>. Inference, unverified. |
| Carburettor or early injection (1980s to 1990s) | No mapped effect. Pops come from rich overrun mixture and leaks, so they are occasional and soft. | Engineering Stack Exchange (forum). Inference for the specific cars, unverified. |

For the four snarl engines (all naturally aspirated, 1980s and 1990s, manual gearboxes):

| Profile | Real car | Expected factory behaviour | Confidence |
| --- | --- | --- | --- |
| `flat12` | Ferrari Testarossa | Mechanical injection, no mapped pops. Soft burbles on overrun from a free-flowing twin system, no upshift bang. | Low, no source found for this car |
| `inline4` | Honda B18C | Deceleration fuel cut, small stock exhaust. Near silent stock. Aftermarket exhausts give light pops. | Low, inferred from the inline-4 class |
| `flat6` | Porsche 993 | Motronic, fuel cut on overrun. Mild overrun pops with a sport exhaust, quiet stock. | Low, inferred |
| `v8` | Ford 5.0 cross-plane | Low-order burble. Occasional deep pop on overrun with an open exhaust. No upshift bang. | Low, inferred |

The arcade liberty is to borrow the modern sport-mode behaviour (upshift bang, lift-off crackle) at a low probability, because players expect it from other games (section 3). Keep the probability low for these cars, and let a race exhaust upgrade raise it.

## 3. Racing games

### 3.1 Summary table

| Game | Trigger | Frequency and randomness | Upgrades and tuning | Reception | Source grade |
| --- | --- | --- | --- | --- | --- |
| Forza Horizon 5 | Lift-off, limiter, gear cracks. Backfires and burbles are per-car recorded assets chopped from track recordings. | Each backfire "has a unique pitch every single time" (press on the A80 Supra demo). | Modular upgrade audio for intake, exhaust and forced induction. Anti-lag (March 2023) fires per cylinder when throttle is below 100 per cent and turbo speed drops, forced to maximum under launch control. | "The popping is CONSTANT", "for supercars that already backfire with race exhaust alone... it's overkill". Pops drop to one per lift after upgrades (bug reports). | Official, press, forum |
| Forza Horizon 6 (May 2026) | Same triggers. | Not stated. | "Upgraded modular systems including turbo and backfires." | Players sceptical before release. | Official |
| Forza Motorsport (Turn 10) | Per-car question: "Does this car burble or backfire when decelerating? If it does, how often and what physics parameters should drive this?" | Driven by physics parameters per car. | Not stated. | Not found. | Press (Nick Wiswell interview) |
| Gran Turismo 7 | Upshift pops on some cars, overrun pops on some. | Fixed per car. | Rev-limit upgrades change pitch on about half the cars (forum count). | Players ask for stock pops on hot hatches and DCT upshift bangs, and complain of "firecracker" pops and of cars that pop but should not. Polyphony added stock pops to the Elantra N in an update (forum report). | Forum |
| Need for Speed Heat (2019) | Lift-off and revving in the garage. | Slider. | Exhaust Sound: Tone, Timbre, Overrun ("subtle" to "raw"), Pipe Resonance. Sound only, no performance change. | Praised as innovative. | Press, wiki |
| Need for Speed Unbound (2022) | Same, plus "Samples" that respond to gear changes and speed. | Slider. | Overrun slider plus a Forced Induction slider. Criterion says "Hyperreal Sound Design" over realism. | Not found. | Official, press |
| Assetto Corsa | Throttle drop inside an RPM window. | `sounds.ini` `[BACKFIRE]`: `MAXGAS`, `MINRPM`, `MAXRPM`, `TRIGGERGAS`, `VOLUME_IN`, `VOLUME_OUT`, `VOLUME_SCALE_OUT`. FMOD events `backfire_ext` and `backfire_int` with a `throttle` parameter. | Modders replace the FMOD bank. Custom Shaders Patch scales backfire volume and pitch. | Not found. | Official (modding docs) |
| rFactor 2 | Event based. A `random` event may hold empty children "to create an event that only plays sometimes". `doNotRestart` stops a burst retriggering while it plays. | Random pool with empty slots as probability. | Content authored per car. | Not found. | Official (Studio 397 docs) |
| Codemasters F1 2010 | Heavy braking into corners. | Recorded trackside at Jerez, layered on the granular engine. | None. | Not found. | Press (MCV) |
| Codemasters GRID 2 | Overrun pops recorded as percussive transients. | Recorded per car. | None stated. | Not found. | Press (Fast and Wide) |
| Project CARS and Project CARS 2 | Crackles and pops kept inside crossfaded loops, because they "confuse and scramble the tracking within a granular synthesis method". FMOD plus REV in PC2. | In the loops. | None stated. | Not found. | Press (Designing Sound, MCV) |
| Burnout series | Boost and sense of speed, "Infinite Gear" model. No backfire system described. | Not applicable. | Not applicable. | Not applicable. | Press (Designing Sound) |
| The Crew Motorfest | No developer source found. | Unknown. | Unknown. | Players praise its less compressed car audio. | Forum |

### 3.2 Forza Horizon 5 and 6

Playground Games records every car on track and chops "all of the gear cracks, all the limiter samples, and all of the idles" for the granular engine, and asks drivers to produce "the ups and downs and the shifts and the Pops and bangs" for cinematic assets (Fraser Strachan, Tonebenders podcast, press) <https://tonebenderspodcast.com/184-forza-horizon-5-with-fraser-strachan/2/>. Each Pro Tools session has a fixed section for "the sweeps the backfires, the burbles" (same source). The team moved to 32-bit recorders because a car "would spit out fire or backfire and cause really loud pops that we weren't necessarily aware was going to happen" (A Sound Effect, press) <https://www.asoundeffect.com/forza-5-game-audio/>. Jalopnik heard that on the demo Supra "the pop of each backfire has a unique pitch every single time" (press) <https://www.jalopnik.com/forza-horizon-5s-car-audio-is-so-good-it-renders-other-1847280213/>.

Anti-lag arrived with the Midnights at Horizon update on 28 March 2023 for every turbocharged car, "producing awesome flame effects and fire-cracker audio" (Forza, official) <https://forza.net/news/forza-horizon-5-midnights>. Playground's Chris Phillips described the trigger: "We are looking at the turbo RPM, when throttle positions is less than 100% and the turbo speed decreases the system fires (per cylinder, which makes the effect different for different engine types). When Launch Control is engaged the system is also forced to maximum effect" (Forza forums transcript of a developer Q and A, official) <https://forums.forza.net/t/fh5-rally-adventure-expansion-now-available/587256?page=24>. More than 630 cars could fit it.

Reception (forum): a 1132-point Reddit post on the anti-lag drew "I personally hate it", "literally 2 minutes was all I could take. The popping is CONSTANT", and "for supercars that already backfire with race exhaust alone... it's overkill" <https://www.reddit.com/r/ForzaHorizon/comments/1lcxyaj/this_anti_lag_is_crazy/>. A poll thread asked for adjustable backfiring because "it just pops way too much" <https://www.reddit.com/r/ForzaHorizon/comments/125yl58/do_you_think_that_antilag_should_have_adjustable/>. Separately, players reported that stock cars give "10-20 bangs" per lift and upgraded cars "2-3" or one, because upgrade exhausts "replace certain sounds with generic ones" (Steam and Reddit, forum) <https://steamcommunity.com/app/1551360/discussions/0/5250637387546673881/>, <https://www.reddit.com/r/ForzaHorizon/comments/rpa7qx/backfires_pops_and_bangs_gone_after_upgrades/>. Players also asked that modified cars "backfire and pop like mad when you're jamming the throttle in neutral" (Forza forums) <https://forums.forza.net/t/car-sounds/534734?page=6>. Both directions of complaint exist: too much on anti-lag supercars, too little on upgraded cars.

Forza Horizon 6 lists "upgraded modular systems including turbo and backfires" as feature 8 of 10 (Forza, official) <https://forza.net/news/forza-horizon-6-features>.

### 3.3 Forza Motorsport

Nick Wiswell (Turn 10) lists the design questions per car, including "Does this car 'burble' or 'backfire' when decelerating? If it does, how often and what physics parameters should drive this?" (Designing Sound, press) <https://designingsound.org/2014/08/11/vehicle-engine-design-project-cars-forza-motorsport-5-and-rev/>. The 2014 Forza Horizon 2 team also said: "We go to great detail to model the sounds of any inconsistencies that are in there as well, which is really important, especially in the decel[eration] as well, with exhaust pops and stuff" (IGN, press) <https://za.ign.com/forza-horizon-2/75868/feature/forza-horizon-2-dev-if-it-looks-stunning-then-it-should-sound-stunning-ign-first>.

### 3.4 Gran Turismo 7

Only forum evidence was found. The GTPlanet car sounds thread (forum) <https://www.gtplanet.net/forum/threads/the-gt7-car-sounds-thread.435519/> reports that Polyphony added stock pops and bangs and upshift pops to the Hyundai Elantra N TC in a Spec III era update, asks for DCT upshift bangs on the Golf GTI and A45, says that the AMG GT S pops sound "like little firecrackers", and says that the 2007 Audi R8 V8 and the SLR McLaren pop in game but should not. One poster prefers "the Gran Turismo approach" of few pops to Need for Speed's "louder" style. The official 1.71 notes (official) <https://www.gran-turismo.com/us/gt7/news/00_3638095.html> change upshift simulation and downshift behaviour but say nothing about pops. The GDC 2024 talk by Polyphony exists but its content was not fetched (GDC Vault, official, unverified for pops) <https://gdcvault.com/play/1034189/-Gran-Turismo-7-Sound>.

### 3.5 Need for Speed Heat and Unbound

Heat's Exhaust Sound menu has four sliders. "Overrun - Adjusts the amounts of pops and bangs of the exhaust whether to be more subtle (less popping) or raw (more and aggressive popping)" (Need for Speed wiki, community documentation) <https://needforspeed.miraheze.org/wiki/Visual_customization_(Heat)>. GTPlanet described the launch video: overrun "ranging from subtle to raw" (press) <https://www.gtplanet.net/exhaust-tuning-will-make-cars-sing-in-need-for-speed-heat/>. Players report that the slider is sound only (Reddit, forum) <https://www.reddit.com/r/NFSHeat/comments/o4qjzy/can_someone_explain_how_the_exhaust_tuning_works/>.

Unbound kept the sliders and added "Forced Induction" (Need for Speed fandom wiki, community documentation) <https://nfs.fandom.com/wiki/Need_for_Speed:_Unbound/Style/Exhaust_Sound>. EA's Under the Hood post (official) says the team "overhauled the Exhaust Sounds from Need for Speed Heat to give you more control and personalization by adding settings for Timber, Tone, Overrun, and strength of the Forced Induction" <https://www.reddit.com/r/needforspeed/comments/yfqg25/nfs_unbound_under_the_hood_show_up_and_show_off/>. Criterion's design pillar was "Hyperreal Sound Design", and a "Samples" layer responds "to things like gear changes and speed" (80.lv, press) <https://80.lv/articles/a-deep-dive-into-need-for-speed-unbound-s-sound-design>.

### 3.6 Assetto Corsa and rFactor 2

Assetto Corsa gates each car's backfire in `data/sounds.ini`: `MAXGAS` (no backfire above this throttle), `MINRPM` and `MAXRPM` (the RPM window), `TRIGGERGAS` (the throttle-change threshold that fires it) and three volume factors (Assetto Corsa Japan LABS reference, community documentation of an official file) <https://labs.assettocorsa.jp/documents/reference/acd/sounds>. The FMOD bank has `backfire_ext` and `backfire_int` events with a `throttle` parameter, and Custom Shaders Patch exposes `BACKFIRE_EXT` and `BACKFIRE_INT` volume and pitch (CSP documentation) <https://docs.assetto.cn/en/car/visual/audio>. Modders put backfire samples in a Multi Sound in those events (Assetto Corsa Mods, forum) <https://assettocorsamods.net/threads/can-i-edit-one-part-of-an-engine-sound.1197/>.

rFactor 2's sound engine is event based. Its `random` event type "Randomly picks a contained event to play it back each time it gets fired ... Contained events may be empty or with invalid samples in order to create an event that only plays sometimes", and a `burst` has `doNotRestart` to prevent a retrigger while it plays (Studio 397 documentation, official) <https://docs.studio-397.com/display/DG/Using+the+new+sound+engine>.

### 3.7 Codemasters, Project CARS, Burnout

F1 2010 added "crackling and backfire for those ferocious F1 sonics created as the car brakes heavily into tough corners", recorded at Jerez (MCV, press) <https://mcvuk.com/business-news/publishing/heard-about-f1-2010/>. For GRID 2 Chris Jojo treated "the overrun pops and detonations" as percussive events "not too far removed from the wallop of a snare hit", and chose drum microphones for them (Fast and Wide, press) <https://www.fast-and-wide.com/faw-news/4887-grid-2>. For Project CARS, Greg Hill kept "crackles and pops" inside crossfaded loops because they would "confuse and scramble the tracking within a granular synthesis method" (Designing Sound, press). Project CARS 2 used FMOD and REV (MCV, press) <https://mcvuk.com/development-news/heard-about-project-cars-2/>. Burnout's audio centred on boost and an "Infinite Gear" model with no backfire system described (Designing Sound, press) <https://designingsound.org/2017/08/31/burnout-a-sound-design-retrospective-with-ben-minto/>.

## 4. Audio middleware practice

| Tool | Variation pool | Probability | Avoid repeat | Spacing and cool-down | Source |
| --- | --- | --- | --- | --- | --- |
| Wwise Random Container | Child objects with weights (a weight is the number of chances out of the sum). | Weights. | Standard or Shuffle mode. "Avoid repeating last x played" excludes the last x objects. Scope Global or Game object. | Transitions of type Delay or Trigger rate between children. | Audiokinetic documentation (official) <https://www.audiokinetic.com/en/library/edge/?id=creating_random_container>, <https://www.audiokinetic.com/fr/public-library/2025.1.7_9143/?id=random_sequence_container_property_editor> |
| FMOD Multi Instrument | Playlist of instruments. | Play percentage per entry. | Shuffle mode: with at least three entries and no play percentages, "no instrument in that playlist will be selected twice in a row". | Not in the instrument. | FMOD Studio manual (official) <https://fmod.com/docs/2.03/studio/instrument-reference.html> |
| FMOD Scatterer Instrument | Playlist. | Spawn rate. | Shuffle by default. | Min and max spawn interval, spawn total, polyphony cap with spawn stealing, per-spawn volume and pitch randomisation. | FMOD Studio manual (official) <https://www.fmod.com/docs/2.03/studio/working-with-instruments.html> |
| FMOD event macros | Not applicable. | Not applicable. | Not applicable. | A Cooldown macro sets the minimum time between instances of an event, and Max Instances caps them. Unverified: the reference page did not fetch. | FMOD Studio manual, unverified |
| rFactor 2 | `random` event. | Empty children. | Not stated. | `doNotRestart`. | Studio 397 (official) |
| Assetto Corsa | FMOD Multi Sound in `backfire_ext`. | Game-side `TRIGGERGAS` and RPM window. | FMOD shuffle. | Game-side. | Official modding docs |
| REV (Crankcase Audio) | Granular engine model. "Full model (accel, decel, idle and pops)". | Not documented. | Not documented. | Not documented. | Crankcase FAQ (official) <https://www.crankcaseaudio.com/faq> |

Two implementation articles describe the trigger side. An Audiokinetic blog on REV (official blog, a user project) names three backfire sounds (stable flame under heavy load, burst backfire on a sudden throttle close, anti-lag on a shift) and fires its backfire "only when the engine speed drops below 4,100 RPM" <https://www.audiokinetic.com/ko/blog/make-racing-engine-sound-based-on-rev2/>. Arcella Sound (vendor blog) intercepts "a Gear_Shift event from the game engine" to "trigger procedural exhaust pops (backfires), transmission clunks, and momentary ducking of the continuous engine loops" <https://www.arcellasound.com/post/the-vacuum-cleaner-problem-architecting-aaa-vehicle-audio-in-wwise-and-fmod>. A Wwise racing tutorial fires an exhaust sound on every downshift except into idle (Audiokinetic blog, official) <https://www.audiokinetic.com/en/blog/building-a-car-racing-system-using-wwise/>.

Distilled rules that every tool shares:

1. A trigger from game state (gear event, throttle drop inside an RPM window), not from a timer.
2. A weighted pool and a shuffle that forbids the same variant twice in a row.
3. A minimum interval, a polyphony cap and a do-not-restart gate.
4. Per-instance random gain and pitch.
5. A probability so that some triggers play nothing.

## 5. Recommendations for snarl

### 5.1 Event model

Replace the single overrun trigger with four, each with its own probability, count, kinds and cool-down. All logic runs in the control block that `pops.control` already uses. The pop events, slots and kinds do not change, so the audio-path cost does not change.

| Event | Condition (snarl computes from the snapshot) | Probability | Pops per event | Kinds and order | Delay after the trigger | Cool-down |
| --- | --- | --- | --- | --- | --- | --- |
| Upshift bang | `ShiftUp` counter advanced, throttle before the shift above 0.8, RPM before the shift above 0.75 of redline | 0.35 | 1 | Boom 0.6, pop 0.4 | 20 to 60 ms (inside the torque cut) | 1.5 s |
| Downshift blip | `ShiftDown` counter advanced, RPM after the shift above 0.5 of redline, throttle below 0.2 | 0.25 | 1 to 2 | Pop 0.6, burble 0.4 | 80 to 200 ms (after the blip), second pop 110 to 264 ms later | 1.0 s |
| Lift-off from high RPM | Throttle falls by more than 0.5 within 150 ms, RPM above 0.6 of redline, `Overrun` set, warm engine | 0.6 at first lift, then the existing halving | 1 to 3 in a run | First event crackle 0.5 or boom 0.5, later events pop 0.6 or burble 0.4 | 150 to 400 ms, then 110 to 264 ms inside the run | Run ends 1.5 to 2.5 s after the lift (the factory countdown). 2.0 s before the next run. |
| Long overrun | `Overrun` set for more than 2.5 s, RPM above 1.65 times idle | 0.1 per second | 1 | Burble only | Random, 0.3 to 1.5 s | 3.0 s |

Global rules: at most one run active, a global cool-down of 0.7 s between events from different triggers, and a cap of six events per 10 s. The upshift bang and the first lift-off event may each override the global cool-down once, because they mark the player's action.

Target rate: six to twelve audible pops per minute of hard driving, against a factory sport-mode car that bangs on most hard upshifts and crackles once per lift. A long coast gives a burble every ten seconds or so. Confidence: medium on the probabilities, which are starting points for the harness, high on the structure, which matches the Focus RS state machine, the Assetto Corsa gate and the Forza anti-lag trigger.

Why these conditions:

- The upshift gate on pre-shift throttle and RPM follows the BMW and PistonHeads descriptions: ignition cut happens only on fast, high-RPM, high-load shifts. A part-throttle shift stays silent.
- The downshift pops follow the blip, not the shift, and need the exhaust open, which the RPM floor stands in for (FerrariChat on bypass valves).
- The lift-off rate gate copies the Focus RS "APP Rate must exceed a rate to trigger" and Assetto Corsa `TRIGGERGAS`. A slow lift gives nothing.
- The countdown copies "a configurable countdown before it will become inactive" and the owner report that pops stop in the same gear. The existing 2.5 s halving stays as the shape inside the window.
- Burble on long overrun keeps a little life in a coast without the crackle that the user hears as too much.

### 5.2 Randomness without repetition

- Kinds: draw from a shuffle bag per trigger with the profile weights, and forbid the same kind twice in a row inside a run (Wwise "avoid repeating last 1", FMOD shuffle). Allow the boom to crackle and crackle to boom alternation, which the pack shows (crackles.md section 10).
- Timing: keep the golden-ratio low-discrepancy sequence for intervals and add it to the trigger delays. Jitter each interval by plus or minus 30 per cent.
- Level and body: randomise each event's gain by plus or minus 3 dB and the body frequency by plus or minus 8 per cent, as Forza's per-pop pitch variation and the Wwise randomisers do. Randomise the sub-event count inside the existing `subs` and `subsRange`.
- Probability decay: after each fired event in a run, multiply the next probability by 0.6 so that a three-pop run is rare and a one-pop run is common.
- Skips: a trigger that fails its probability draw plays nothing and still starts its cool-down, so two fast shifts never both bang.

### 5.3 Profile and upgrade character

Add one value to `[pops]`, `character`, with three named levels. A car upgrade in the game selects the level. The level scales the table above:

| Level | Density multiplier | Gain | Upshift probability | Kind shift | Real-world match |
| --- | --- | --- | --- | --- | --- |
| `stock` | 0.5 | -6 dB | 0.1 | Burble and pop up, crackle and boom down | Filtered or standard exhaust, normal mode |
| `sport` (default) | 1.0 | 0 dB | 0.35 | Profile weights | Factory sport mode, open valves |
| `race` | 1.8 | +3 dB | 0.6 | Crackle and boom up | Free-flowing race exhaust, decat |

Cut the base `density` of each profile by about half from today's values (0.4, 0.05, 0.3, 0.25) and let `race` restore the current loudness. This matches the user's request and the Forza finding that race exhausts "already backfire" enough without anti-lag. Keep the per-engine kind weights, because the user's four engines differ by exhaust (two silencers and large bores on the V8, a free pair of pipes on the flat-12), not by crank layout, and the sources tie pop character to the exhaust.

A turbo profile, when one exists, gets a fifth trigger, anti-lag, that fires on every lift with a bright bang. Keep it out of the supercar set.

### 5.4 What the game must send

Nothing new is required. snarl already receives `RPM`, `Throttle`, `Gear`, the `Overrun` and `FinalGear` flags and the `ShiftUp`, `ShiftDown` and `Limiter` counters. snarl keeps a 150 ms throttle history and the pre-shift RPM itself. Two optional additions:

1. An exhaust character byte in the snapshot or in the profile selection call, so that a car upgrade changes the level at run time.
2. A `Brake` flag, if the game wants more downshift pops under braking into a corner, as F1 2010 did. Not needed for the first pass.

### 5.5 Efficiency

The new logic adds a few comparisons and one random draw per control block, plus a small ring of throttle samples. No new DSP. Pop events reuse the four existing slots and the `busyLen` gate, which already acts as a do-not-restart rule.

## 6. Gaps and unverified items

- No source gives pops per minute for any real car. The rates in section 5 are design targets.
- Gran Turismo 7 evidence is forum only. The GDC 2024 Polyphony talk was not fetched.
- The Crew Motorfest has no developer source in this report.
- The FMOD Cooldown and Max Instances event macros come from memory. The reference page did not fetch.
- The factory behaviour of the Testarossa, B18C, 993 and Fox-body 5.0 is inferred from their era, not sourced.
- The EV-Engine pop probabilities (35 and 25 per cent) are a vendor's synthetic preset values, not measurements.

## 7. Sources

Official and documentation:

- Forza, Midnights at Horizon update notes: <https://forza.net/news/forza-horizon-5-midnights>
- Forza, Rally Adventure announcement: <https://forza.net/news/forza-horizon-5-rally-adventure>
- Forza, Rally Adventure FAQ: <https://support.forza.net/hc/en-us/articles/14234333386643-Forza-Horizon-5-Rally-Adventure-FAQ>
- Forza forums, developer Q and A transcript: <https://forums.forza.net/t/fh5-rally-adventure-expansion-now-available/587256?page=24>
- Forza, Forza Horizon 6 features: <https://forza.net/news/forza-horizon-6-features>
- Gran Turismo 7 update 1.71: <https://www.gran-turismo.com/us/gt7/news/00_3638095.html>
- GDC Vault, Gran Turismo 7 sound design: <https://gdcvault.com/play/1034189/-Gran-Turismo-7-Sound>
- EA, Need for Speed Unbound Under the Hood (Reddit mirror): <https://www.reddit.com/r/needforspeed/comments/yfqg25/nfs_unbound_under_the_hood_show_up_and_show_off/>
- Assetto Corsa Japan LABS, sounds.ini reference: <https://labs.assettocorsa.jp/documents/reference/acd/sounds>
- Custom Shaders Patch audio options: <https://docs.assetto.cn/en/car/visual/audio>
- Studio 397, rFactor 2 sound engine: <https://docs.studio-397.com/display/DG/Using+the+new+sound+engine>
- Audiokinetic, Random Container: <https://www.audiokinetic.com/en/library/edge/?id=creating_random_container>
- Audiokinetic, Random and Sequence Container property editor: <https://www.audiokinetic.com/fr/public-library/2025.1.7_9143/?id=random_sequence_container_property_editor>
- Audiokinetic blog, REV racing engine: <https://www.audiokinetic.com/ko/blog/make-racing-engine-sound-based-on-rev2/>
- Audiokinetic blog, car racing system: <https://www.audiokinetic.com/en/blog/building-a-car-racing-system-using-wwise/>
- FMOD Studio, instrument reference: <https://fmod.com/docs/2.03/studio/instrument-reference.html>
- FMOD Studio, working with instruments: <https://www.fmod.com/docs/2.03/studio/working-with-instruments.html>
- Crankcase Audio FAQ: <https://www.crankcaseaudio.com/faq>
- Crankcase Audio REV wiki: <https://github.com/CrankcaseAudio/REVWiki>

Press and interviews:

- A Sound Effect, Forza Horizon 5 audio: <https://www.asoundeffect.com/forza-5-game-audio/>
- Tonebenders podcast 184, Fraser Strachan: <https://tonebenderspodcast.com/184-forza-horizon-5-with-fraser-strachan/2/>
- Jalopnik, Forza Horizon 5 audio: <https://www.jalopnik.com/forza-horizon-5s-car-audio-is-so-good-it-renders-other-1847280213/>
- IGN, Forza Horizon 2 audio: <https://za.ign.com/forza-horizon-2/75868/feature/forza-horizon-2-dev-if-it-looks-stunning-then-it-should-sound-stunning-ign-first>
- Designing Sound, vehicle engine design (Project CARS, Forza, REV): <https://designingsound.org/2014/08/11/vehicle-engine-design-project-cars-forza-motorsport-5-and-rev/>
- Designing Sound, Burnout retrospective: <https://designingsound.org/2017/08/31/burnout-a-sound-design-retrospective-with-ben-minto/>
- MCV, F1 2010: <https://mcvuk.com/business-news/publishing/heard-about-f1-2010/>
- MCV, Project CARS 2: <https://mcvuk.com/development-news/heard-about-project-cars-2/>
- Fast and Wide, GRID 2 recording: <https://www.fast-and-wide.com/faw-news/4887-grid-2>
- 80.lv, Need for Speed Unbound sound design: <https://80.lv/articles/a-deep-dive-into-need-for-speed-unbound-s-sound-design>
- GTPlanet, Need for Speed Heat exhaust tuning: <https://www.gtplanet.net/exhaust-tuning-will-make-cars-sing-in-need-for-speed-heat/>
- MotorTrend, anti-lag: <https://www.motortrend.com/how-to/what-is-anti-lag>
- Motor1, auto rev matching: <https://www.motor1.com/features/707050/how-auto-rev-match-works/>
- Autoblog, 2021 F-Type R exhaust: <https://www.autoblog.com/2020/08/04/2021-jaguar-f-type-r-exhaust-test/>
- CarThrottle, petrol particulate filters: <https://www.carthrottle.com/news/what-are-petrol-particulate-filters-and-why-do-they-make-cars-sound-worse>
- Autotrader UK, why cars sound worse in 2020: <https://www.autotrader.co.uk/content/news/this-is-why-cars-sound-terrible-in-2020>
- Motor Authority, engine sound by type: <https://www.motorauthority.com/news/1144083_science-why-different-types-of-engines-sound-the-way-they-do>
- Car and Driver, engine sound by type: <https://www.caranddriver.com/features/a15107374/this-is-why-various-engine-types-sound-so-different-feature/>
- Sgcarmart, pop and bang maps: <https://www.sgcarmart.com/articles/advice/what-they-dont-tell-you-your-pop-and-bang-map-isnt-anti-lag-and-it-isnt-doing-anything-for-your-cars-performance-31071>

Tuner sources:

- StrikeEngine, Focus RS burble strategy: <https://strikeengine.com/exhaust-burbelspopscrackles-how-its-done/>
- xAutomotive, BMW shift sounds: <https://www.xautomotive.com/blogs/news/the-fart-secret>
- HP Academy, pops on gear change: <https://www.hpacademy.com/q-and-a/pops-and-bangs-why-it-occurs-when-changing-gears/>
- HP Academy forum, shift cut: <https://www.hpacademy.com/forum/engine-building-fundamentals/show/gear-shift-with-ignition-cut/>
- Equilibrium Tuning, burbles: <https://eqtuning.com/blogs/technical-write-ups/burbles-pops-and-bangs-crackles>
- Thanas, tuning for crackles: <https://thanas.co.za/blog/25-tuning-for-exhaust-crackles/body>
- GOW Tuning, pops and bangs: <https://gowtuning.com/pop-bang/>
- Eagle Tuning, pop and bang FAQ: <https://blog.eagletuning.com/pop-bang-tuning-faqs/>
- EV-Engine, cross-plane and flat-plane presets: <https://ev-engine.com/blog/crossplane-and-flat-plane-v8>

Forum sources:

- PistonHeads, how manufacturers map in bangs: <https://www.pistonheads.com/gassing/topic.asp?f=66&h=0&t=1521874>
- PistonHeads, F-Type crackle removed after update: <https://www.pistonheads.com/gassing/topic.asp?f=85&h=0&i=140&t=1890689>
- F-Type forums, exhaust note: <https://ftypeforums.co.uk/threads/exhaust-note.3641/>
- F-Type forums, particulate filter: <https://ftypeforums.co.uk/threads/petrol-particulate-filter.6208/>
- FerrariChat, throttle blip: <https://www.ferrarichat.com/forum/threads/throttle-blip.388211/>
- Engineering Stack Exchange, why supercars backfire: <https://engineering.stackexchange.com/questions/14831/why-do-fuel-injected-supercars-still-backfire>
- Reddit, Forza anti-lag reception: <https://www.reddit.com/r/ForzaHorizon/comments/1lcxyaj/this_anti_lag_is_crazy/>
- Reddit, adjustable anti-lag poll: <https://www.reddit.com/r/ForzaHorizon/comments/125yl58/do_you_think_that_antilag_should_have_adjustable/>
- Reddit, pops gone after upgrades: <https://www.reddit.com/r/ForzaHorizon/comments/rpa7qx/backfires_pops_and_bangs_gone_after_upgrades/>
- Steam, how to fix pops and bangs: <https://steamcommunity.com/app/1551360/discussions/0/5250637387546673881/>
- Forza forums, car sounds: <https://forums.forza.net/t/car-sounds/534734?page=6>
- Forza forums, pops not working after tuning: <https://forums.forza.net/t/exhaust-pops-and-bangs-not-properly-working/539327>
- Reddit, FH6 audio fixes: <https://www.reddit.com/r/ForzaHorizon/comments/1r24a2s/details_on_audio_fixes_in_fh6/>
- GTPlanet, GT7 car sounds thread: <https://www.gtplanet.net/forum/threads/the-gt7-car-sounds-thread.435519/>
- Reddit, NFS Heat exhaust tuning: <https://www.reddit.com/r/NFSHeat/comments/o4qjzy/can_someone_explain_how_the_exhaust_tuning_works/>
- Need for Speed wiki, Heat customisation: <https://needforspeed.miraheze.org/wiki/Visual_customization_(Heat)>
- Need for Speed fandom wiki, Unbound exhaust sound: <https://nfs.fandom.com/wiki/Need_for_Speed:_Unbound/Style/Exhaust_Sound>
- Assetto Corsa Mods, editing backfire events: <https://assettocorsamods.net/threads/can-i-edit-one-part-of-an-engine-sound.1197/>
- Arcella Sound (vendor blog), vehicle audio architecture: <https://www.arcellasound.com/post/the-vacuum-cleaner-problem-architecting-aaa-vehicle-audio-in-wwise-and-fmod>
