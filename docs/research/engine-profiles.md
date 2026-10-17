# Engine profiles: reference facts for the four snarl sound models

Date: 2026-10-17. Scope: the engine facts that decide the sound of the four initial profiles. Code internals are out of scope.

Every material number carries a source. Where sources disagree, both values appear. Items marked "unverified" have no primary source yet.

## Summary table

| Parameter | Flat-12: Ferrari Testarossa F113 | Inline-4: Honda B18C5 (Integra Type R) | Flat-6: Porsche M64/05 and M64/21 (911 993) | Cross-plane V8: Ford 5.0 HO (Fox Mustang) |
| --- | --- | --- | --- | --- |
| Years | 1984 to 1991 | 1997 to 2001 (US), 1995 to 2001 (JDM B18C) | 1994 to 1998 | 1987 to 1993 |
| Layout | 180-degree V12, shared crankpins (not a boxer) | Inline-4, transverse | True boxer flat-6, air cooled, rear | 90-degree V8, cross-plane crank, OHV |
| Displacement | 4943 cc | 1797 cc | 3600 cc | 4942 cc (302 cu in) |
| Bore x stroke | 82 x 78 mm | 81 x 87.2 mm | 100 x 76.4 mm | 101.6 x 76.2 mm (4.00 x 3.00 in) |
| Compression | 9.3:1 (Euro), 8.8:1 (US) | 10.6:1 (US), 11.1:1 (JDM) | 11.3:1 | 9.0:1 |
| Valves | DOHC, 4 per cylinder, 48 | DOHC VTEC, 4 per cylinder, 16 | SOHC, 2 per cylinder, 12 | OHV, 2 per cylinder, 16 |
| Induction | Bosch K-Jetronic (KE-Jetronic on cat cars) | PGM-FI, single-port manifold | Bosch Motronic, two-stage resonance intake, VarioRam from 1996 | Sequential EFI, speed density then mass air from 1989 |
| Firing order | 1-9-5-12-3-8-6-10-2-7-4-11 (512 BB manual, see caveat) | 1-3-4-2 | 1-6-2-4-3-5 | 1-3-7-2-6-5-4-8 |
| Firing interval | 60 degrees even; 120 degrees even per bank | 180 degrees even | 120 degrees even; 240 degrees even per bank | 90 degrees even; 90-180-270-180 per bank |
| Idle | unverified, about 1000 rpm | 800 +/- 50 rpm | 800 +/- 40 rpm | 675 rpm (M/T) |
| Peak power | 390 PS at 6300 rpm | 195 hp at 8000 rpm | 272 PS at 6100 (M64/05), 285 PS at 6100 (M64/21) | 225 hp at 4200 rpm |
| Peak torque | 490 Nm at 4500 rpm | 130 lb-ft at 7500 rpm | 330 Nm at 5000 (M64/05), 340 Nm at 5250 (M64/21) | 300 lb-ft at 3200 rpm |
| Redline | 6800 rpm | 8400 rpm | 6800 rpm | about 6000 on the tachometer (unverified) |
| Rev limiter | unverified | 8500 rpm fuel cut (US), 8900 (JDM, weak source) | 6700 +/- 20 rpm governed | 6250 rpm fuel cut |
| Exhaust | Two bank systems into one silencer, four tailpipes, two cats on cat cars | 4-2-1 header, one cat, one silencer with a third chamber, one tailpipe | Per-bank heat exchangers into a centre mixing chamber, two cats, two silencers, two tailpipes | Shorty headers, four-cat H-pipe, two silencers, two tailpipes, 2.25 in pipe |
| Dominant order | 6th (3rd per bank) | 2nd | 3rd (1.5 per bank) | 4th, with 0.5 and 1.5 order sidebands |

## 1. Flat-12: Ferrari Testarossa, Tipo F113 (priority)

### Identity

| Item | Value | Source |
| --- | --- | --- |
| Engine codes | F113 A 000 (Euro, no cat, to 1986), F113 A 040 (US and Japan, cat), F113 A 046 (CH and S), F113 B 000 (Euro with cat, from 1986), F113 B 046 | [ferrari-data.cz](http://www.ferrari-data.cz/ferrari_testarossa.html) |
| Car and years | Ferrari Testarossa, 1984 to 1991, 7177 built | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa) |
| Layout | Rear, longitudinal, 180-degree V12. Opposed con-rods share one crankpin, so opposed pistons move in the same direction. Forghieri: "Please, don't call it boxer." | [Wikipedia, Ferrari flat-12](https://en.wikipedia.org/wiki/Ferrari_flat-12_engine) |
| Bore x stroke | 82 x 78 mm | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa) |
| Displacement | 4943.04 cc, 411.92 cc per cylinder | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa) |
| Compression | 9.3:1 (Ferrari.com), 9.2:1 (workshop manual listing, Automotive Masterpieces), 8.8:1 US | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa), [Repairloader](https://www.repairloader.com/manual.php/19ba41), [ferrari-data.cz](http://www.ferrari-data.cz/ferrari_testarossa.html) |
| Valvetrain | Twin belt-driven overhead camshafts per bank, four valves per cylinder, 48 valves, bucket tappets with shims | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa) |
| Induction | Bosch K-Jetronic mechanical injection (F113 A), KE-Jetronic on cat cars; Marelli Microplex MED 120 B ignition, one plug per cylinder | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa), [Red Headed](https://www.red-headed.com/testarossa_specs.html) |
| Lubrication | Dry sump | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa) |

### Firing order and intervals

The crank is a six-throw, 120-degree crank with paired rods on each throw, the same as a V12. Each bank fires as an inline-six at 120-degree intervals, and the two banks are 60 degrees out of phase, so the whole engine fires every 60 degrees ([FerrariLife thread](https://www.ferrarilife.com/threads/v12-cylinder-numbering-firing-orders.11676/)).

Ferrari numbers cylinders in a U: 1 to 6 on the right bank from the front, 7 to 12 on the left bank from the rear, so 12 is left front ([FerrariLife thread](https://www.ferrarilife.com/threads/v12-cylinder-numbering-firing-orders.11676/)).

The 512 BB owner's manual gives 1-9-5-12-3-8-6-10-2-7-4-11. Per bank that is 1-5-3-6-2-4 (right) and 9-12-8-10-7-11 (left) ([FerrariChat 512 BB thread](https://www.ferrarichat.com/forum/threads/does-anybody-have-a-firing-order-diagram-for-a-512bb.404258/)). Caveat: this is the 512 BB value. The Testarossa F113 shares the block and crank architecture ([CarThrottle](https://www.carthrottle.com/news/heres-why-ferraris-old-flat-12-isnt-exactly-flat-12-all)), but no Testarossa manual page was read. Treat the F113 firing order as unverified.

Pulse pattern per exhaust bank: even, 120 degrees, three pulses per revolution. The exhaust systems are per bank (see below), so each tailpipe pair behaves like an inline-six exhaust.

### Idle, power, torque, redline

| Item | Value | Source |
| --- | --- | --- |
| Peak power | 287 kW (390 PS) at 6300 rpm (Euro); 380 PS at 5750 rpm (US); 390 PS at 5800 rpm (F113 B) | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa), [ferrari-data.cz](http://www.ferrari-data.cz/ferrari_testarossa.html) |
| Peak torque | 490 Nm at 4500 rpm (Euro); 470 Nm at 4500 rpm (US); 485 Nm at 4800 rpm (F113 B) | [Ferrari.com](https://www.ferrari.com/en-EN/auto/testarossa), [ferrari-data.cz](http://www.ferrari-data.cz/ferrari_testarossa.html) |
| Cat-era Euro power | 370 PS and 451 Nm (1986 to 1989); 381 PS and 470 Nm (1989 to 1991) | [Wikipedia, Testarossa](https://en.wikipedia.org/wiki/Ferrari_Testarossa) |
| Redline | 6800 rpm | [Octane](https://www.octane-magazine.com/articles/features/ferrari-testarossa-daily-driving-a-supercar-legend-in-london/), [ferrari-data.cz](http://www.ferrari-data.cz/ferrari_testarossa.html) |
| Rev limiter | Unverified. The 512 BBi Microplex ignition had a built-in rev limiter, and the Testarossa uses Microplex too, but no limiter rpm was found | [Wikipedia, 512 BB](https://en.wikipedia.org/wiki/Ferrari_512_BB) |
| Idle | Unverified. Octane reports the engine "hunts a little" at idle | [Octane](https://www.octane-magazine.com/articles/features/ferrari-testarossa-daily-driving-a-supercar-legend-in-london/) |
| Power delivery | "really coming alive and belting it out from 5000rpm all the way to the redline at 6800" | [Octane](https://www.octane-magazine.com/articles/features/ferrari-testarossa-daily-driving-a-supercar-legend-in-london/) |
| Valve timing | Unverified. The factory technical manual has a "Valve Timing" and "Timing Data" section, not read | [ManualsLib](https://www.manualslib.com/manual/466980/Ferrari-Testarossa.html) |

### Exhaust

The factory parts table (Table 20) lists four exhaust manifolds (RH front, LH front, RH rear, LH rear), one "exhaust pipe from R.H. manifold to silencer", one "exhaust pipe from L.H. manifold to silencer", two catalytic converters on cat cars, and one exhaust silencer ([Ricambi America](https://www.ricambiamerica.com/car-diagrams/ferrari/v12/testarossa-group/testarossa-1990/exhaust-system-for-us-sa-and-cat-version.html), [Ferrari Philadelphia](https://ferrariphiladelphia.com/store/parts/12-cylinder/testarossa-group/testarossa/table-20)). So each bank has a 6-into-1 path (two 3-into-1 manifolds per bank joined by a bank pipe) into a single shared silencer box. The car has four tailpipes ([Sports Car Bible](https://sportscarbible.com/ferrari/ferrari-testarossa/ferrari-testarossa.html)). Whether the two bank flows mix inside the silencer is unknown (open question). Pipe diameters and lengths are unknown.

### Sound character

- Each bank is an even-firing inline-six on its own pipe. The whole engine is a 6th-order source, 630 Hz at 6300 rpm. The per-bank 3rd order is 50 Hz at 1000 rpm.
- Reviewers describe "a glorious, sonorous, multi-multi-layered thrash" rather than a howl, with power arriving above 5000 rpm ([Hagerty](https://www.hagerty.com/media/market-trends/hagerty-insider/these-three-flavors-of-ferrari-testarossa-have-distinct-personalities-and-values/)). Motor Sport in 1986 heard "a muted growl" and "little engine noise" from inside ([Motor Sport](https://www.motorsportmagazine.com/archive/article/july-1986/88/ferrari-testarossa/)). The Octane car was quiet below 3000 rpm and sang from there up ([Octane](https://www.octane-magazine.com/articles/features/ferrari-testarossa-daily-driving-a-supercar-legend-in-london/)).
- Decat cars are "throaty, full" ([Road & Track](https://www.roadandtrack.com/new-cars/a26535370/ferrari-testarossa-sound/)). Most loud Testarossa videos use aftermarket or decat systems, so do not calibrate to them.
- Overrun: unverified. K-Jetronic is a continuous mechanical injection, so unburnt fuel reaches the exhaust on lift-off more than with sequential EFI. Expect pops on decat cars and few on cat cars. No period road test describing Testarossa overrun was found.

## 2. Inline-4: Honda B18C5, Integra Type R (DC2)

Why this engine: the Type R is the best-documented naturally aspirated performance four of the 1990s, with factory figures for redline, fuel cut, VTEC changeover, cam timing, and exhaust design. Its sound has two distinct states (below and above VTEC changeover), which gives the profile a character the other three lack. Alternatives considered: Toyota 4A-GE (AE86, 1983 to 1987) and Honda B16A (1989 CRX SiR). Both fit the era but have thinner English-language factory data.

### Identity

| Item | Value | Source |
| --- | --- | --- |
| Engine codes | B18C5 (US, 1997 to 2001), B18C Spec R (JDM, 1995 to 2001) | [Project JDM](https://projectjdm.org/wiki/specs/integra-type-r-dc2) |
| Layout | Inline-4, transverse, front-wheel drive | [Honda News 2001 specs](https://acuranews.com/en-US/releases/release-038578562975f17ab17410004c34c8a5-2001-acura-integra-type-r-specifications) |
| Bore x stroke | 81 x 87.2 mm | [Honda News 1998 specs](https://web.archive.org/web/20140122225026/www.honda.com/newsandviews/article.aspx?id=823-en) |
| Displacement | 1797 cc | [Honda News 1998 specs](https://web.archive.org/web/20140122225026/www.honda.com/newsandviews/article.aspx?id=823-en) |
| Compression | 10.6:1 (US B18C5); 11.1:1 (JDM B18C Spec R) | [Honda News 1998 specs](https://web.archive.org/web/20140122225026/www.honda.com/newsandviews/article.aspx?id=823-en), [Wikipedia, Honda B engine](https://en.wikipedia.org/wiki/Honda_B_engine) |
| Rod length, rod to stroke | 137.9 mm, 1.58 | [Wikipedia, Honda B engine](https://en.wikipedia.org/wiki/Honda_B_engine) |
| Valvetrain | DOHC VTEC, 4 valves per cylinder, single belt | [Honda News 2001 specs](https://acuranews.com/en-US/releases/release-038578562975f17ab17410004c34c8a5-2001-acura-integra-type-r-specifications) |
| Induction | PGM-FI sequential, single-port intake manifold, 42 mm runners, throttle bore widened by 2 mm over GS-R | [Honda News 2001 specs](https://acuranews.com/en-US/releases/release-038578562975f17ab17410004c34c8a5-2001-acura-integra-type-r-specifications), [Motor1](https://www.motor1.com/features/740601/integra-type-r-tech-guide/) |

### Firing order and intervals

1-3-4-2, even 180-degree intervals ([AutoZone firing orders](https://www.autozone.com/diy/repair-guides/acura-cl-integra-nsx-rl-rsx-tl-tsx-2001-2006-firing-orders-repair-guide-p-0996b43f80cb0da6)). One bank. Cylinders 1 and 4 fire 360 degrees apart, as do 2 and 3, so a 4-2-1 header pairs 1 with 4 and 2 with 3 and each secondary carries an even 360-degree pulse train.

### Idle, power, torque, redline

| Item | Value | Source |
| --- | --- | --- |
| Peak power | 195 hp SAE net at 8000 rpm (US); 200 PS at 8000 rpm (JDM) | [Honda News 1998 specs](https://web.archive.org/web/20140122225026/www.honda.com/newsandviews/article.aspx?id=823-en), [Project JDM](https://projectjdm.org/wiki/specs/integra-type-r-dc2) |
| UK figures | 187 bhp at 7900 rpm, 131 lb-ft at 7200 rpm (UK); 197 bhp at 8000, 134 lb-ft at 6200 (JDM) | [Rush Magazine](https://www.rushmagazine.co.uk/post/the-ultimate-buying-guide-for-the-honda-integra-type-r-dc2) |
| Peak torque | 130 lb-ft at 7500 rpm (US) | [Honda News 1998 specs](https://web.archive.org/web/20140122225026/www.honda.com/newsandviews/article.aspx?id=823-en) |
| Redline | 8400 rpm | [Honda News 1998 specs](https://web.archive.org/web/20140122225026/www.honda.com/newsandviews/article.aspx?id=823-en) |
| Fuel cut | 8500 rpm (US) | [Honda News 1998 specs](https://web.archive.org/web/20140122225026/www.honda.com/newsandviews/article.aspx?id=823-en) |
| JDM rev limit | 8900 rpm (Wikipedia, citation needed), 8500 rpm (integratyper.org for B18C5) | [Wikipedia, Honda B engine](https://en.wikipedia.org/wiki/Honda_B_engine), [integratyper.org](http://www.integratyper.org/specs/master_comparison.htm) |
| VTEC changeover | 5700 rpm (US); 5800 rpm (JDM 96 spec), 6000 rpm (JDM 98 spec) | [Honda News 1998 specs](https://web.archive.org/web/20140122225026/www.honda.com/newsandviews/article.aspx?id=823-en), [Wikipedia, Honda B engine](https://en.wikipedia.org/wiki/Honda_B_engine) |
| Idle | 800 +/- 50 rpm, M/T neutral (B18C5); 750 +/- 50 for B18B1 and B18C1 | [1998 Integra service manual via carmanualsonline](https://www.carmanualsonline.info/honda-integra-1998-4-g-workshop-manual/?srch=fuel+tank+capacity) |
| Fast idle | 1500 +/- 200 rpm (B18C5) | [1998 Integra service manual via carmanualsonline](https://www.carmanualsonline.info/honda-integra-1998-4-g-workshop-manual/?srch=fuel+tank+capacity) |
| VTEC cam lobe timing (B18C 96 Spec R) | Intake opens 15 degrees BTDC, closes 45 degrees ABDC, lift 11.5 mm. Exhaust opens 45 degrees BBDC, closes 10 degrees ATDC, lift 10.5 mm | [amfeatures B-series table](https://amfeatures.tripod.com/id9.html) |
| Valve diameters | Intake 33 mm, exhaust 28 mm | [amfeatures B-series table](https://amfeatures.tripod.com/id9.html) |

### Exhaust

Honda: "a large-diameter Tri-Y header system, the individual head pipes gently converge into a tapered collector", stainless, feeding "significantly increased pipe diameters, the addition of a third exhaust chamber, and a performance-oriented muffler with a large NSX-style exhaust tip". Flow is up 30 percent over the GS-R ([Honda News 1997 powertrain](https://hondanews.com/en-US/releases/release-398aa72722c1c76a6c1681004c34c9a9-1997-acura-integra-type-r-powertrain)). Tri-Y means 4-2-1. One three-way catalyst ([Honda News 2001 specs](https://acuranews.com/en-US/releases/release-038578562975f17ab17410004c34c8a5-2001-acura-integra-type-r-specifications)). Single tailpipe. The JDM car has a better 4-2-1 manifold and a smaller catalyst ([Rush Magazine](https://www.rushmagazine.co.uk/post/the-ultimate-buying-guide-for-the-honda-integra-type-r-dc2)). Pipe diameters: unknown.

### Sound character

- 2nd-order source. 266.7 Hz at 8000 rpm, 26.7 Hz at 800 rpm idle.
- "A proper screamer", "it'll happily zing all the way to 8,400rpm" ([Rush Magazine](https://www.rushmagazine.co.uk/post/the-ultimate-buying-guide-for-the-honda-integra-type-r-dc2)). The VTEC changeover is audible as a step in induction noise and a hardening of the exhaust note. The single-port manifold and large throttle make induction roar a major part of the sound.
- Overrun: sequential PGM-FI cuts fuel on deceleration, so a stock car pops little. Loud pops come from aftermarket exhausts and tunes. Model overrun as sparse and quiet for this profile.

## 3. Flat-6: Porsche M64/05 and M64/21, 911 Carrera (993)

Why this engine: the 993 is the last air-cooled 911 and has a factory technical specification booklet with bore, stroke, compression, firing order, idle speed, governed speed, and valve timing. It is a true boxer, which contrasts with the Testarossa's shared-crankpin flat-12, and its fan whine and valvetrain noise give it a mechanical layer the other profiles lack. Alternatives considered: 964 M64/01 (1989 to 1994, 250 PS, same bore and stroke, same firing order) and the 3.2 Carrera. The 993 data is the most complete.

### Identity

| Item | Value | Source |
| --- | --- | --- |
| Engine codes | M64/05 (manual, RoW, 1994 to 1995), M64/06 (Tiptronic), M64/07 and /08 (US), M64/21 to /24 (VarioRam, 1996 to 1998) | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf), [964 and 993 engine document](https://data.club911.net/divers/964tech/964doc.pdf) |
| Layout | Air-cooled flat-6, rear, longitudinal, each piston on its own crankpin (true boxer) | [Wikipedia, 993](https://en.wikipedia.org/wiki/Porsche_911_(993)), [Hagerty](https://www.hagerty.com/media/magazine-features/hagerty-magazine/porsches-air-cooled-flat-six-engine-thrived/) |
| Bore x stroke | 100 x 76.4 mm | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |
| Displacement | 3600 cc | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |
| Compression | 11.3:1 | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf), [Berlin Motor Books](https://berlinmotorbooks.de/en/the-porsche-993-flat-six-engine-the-last-air-cooled-boxer/) |
| Valvetrain | SOHC per bank, one intake and one exhaust valve per cylinder, hydraulic lash adjustment, chain driven | [Wikipedia, 993](https://en.wikipedia.org/wiki/Porsche_911_(993)), [Berlin Motor Books](https://berlinmotorbooks.de/en/the-porsche-993-flat-six-engine-the-last-air-cooled-boxer/) |
| Valve sizes (M64/21) | Intake 50 mm, exhaust 43.5 mm | [Berlin Motor Books](https://berlinmotorbooks.de/en/the-porsche-993-flat-six-engine-the-last-air-cooled-boxer/) |
| Induction | Bosch Motronic 2.10 with hot-film air mass meter, two-stage resonance intake; VarioRam variable-length intake from model year 1996 | [Berlin Motor Books](https://berlinmotorbooks.de/en/the-porsche-993-flat-six-engine-the-last-air-cooled-boxer/) |
| Ignition | DME, twin ignition (two plugs per cylinder) with knock control | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |
| Cooling fan | Crank-driven belt fan. The 901 fan ran at 1.3 times crank speed with 11 blades, the 1976 fan at 1.8 times with 5 blades | [Hagerty](https://www.hagerty.com/media/magazine-features/hagerty-magazine/porsches-air-cooled-flat-six-engine-thrived/) |
| Engine weight | 232 kg (Tiptronic 224 kg) | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |

Note: enginecode.uk lists DOHC, 24 valves and 10.4:1 for the M64/08. That page conflicts with the Porsche booklet and Wikipedia and is not used.

### Firing order and intervals

1-6-2-4-3-5 ([993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf), [964 document](https://data.club911.net/divers/964tech/964doc.pdf)). Porsche numbers 1 to 3 on the right bank and 4 to 6 on the left bank. The order alternates banks (R L R L R L), so the whole engine fires every 120 degrees and each bank fires every 240 degrees, evenly. With three cylinders per bank, each heat exchanger carries 1.5 pulses per revolution.

### Idle, power, torque, redline

| Item | Value | Source |
| --- | --- | --- |
| Peak power M64/05 | 200 kW (272 PS) at 6100 rpm | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |
| Peak torque M64/05 | 330 Nm at 5000 rpm | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |
| Peak power M64/21 | 210 kW (285 PS) at 6100 rpm | [Berlin Motor Books](https://berlinmotorbooks.de/en/the-porsche-993-flat-six-engine-the-last-air-cooled-boxer/) |
| Peak torque M64/21 | 340 Nm at 5250 rpm | [Berlin Motor Books](https://berlinmotorbooks.de/en/the-porsche-993-flat-six-engine-the-last-air-cooled-boxer/) |
| US rating | 270 hp at 6100 rpm, 243 lb-ft at 5000 rpm | [Car and Driver 1995 test](https://www.caranddriver.com/reviews/a15141049/1995-porsche-911-carrera-archived-road-test-review/) |
| Redline | 6800 rpm | [Car and Driver 1995 test](https://www.caranddriver.com/reviews/a15141049/1995-porsche-911-carrera-archived-road-test-review/) |
| Governed speed | 6700 +/- 20 rpm | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |
| Idle | 800 +/- 40 rpm (manual), 750 +/- 40 rpm (Tiptronic) | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |
| Power delivery | "a clear wake-up call about 5000 rpm intensifies the charge to the 6800-rpm redline" | [Car and Driver 1995 test](https://www.caranddriver.com/reviews/a15141049/1995-porsche-911-carrera-archived-road-test-review/) |
| Valve timing (964 M64/01, at 1 mm lift) | Intake opens 4 degrees BTDC, closes 56 degrees ABDC. Exhaust opens 45 degrees BBDC, closes 50 degrees ATDC as OCR reads; the exhaust close value looks like an OCR error, unverified | [964 document](https://data.club911.net/divers/964tech/964doc.pdf) |
| Valve timing (993 Carrera, at 1 mm lift) | OCR reads 1 degree BTDC, 60 degrees ABDC, 45 degrees BBDC, 5 degrees ATDC. Unverified, OCR is poor | [993 Technical Specification Booklet](https://raw.githubusercontent.com/anstack/anstack.github.io/master/static/993/Technical%20-%20993%20Technical%20Specification%20Booklet.pdf) |

### Exhaust

Both heat exchangers (one per bank, each a 3-into-1 manifold wrapped for cabin heat) feed a centre mixing chamber. A catalyst element sits on each side of the chamber, and each catalyst feeds a silencer ([Pelican Parts thread, quoting US patent 5,325,666](http://forums.pelicanparts.com/porsche-911-technical-forum/566890-993-exhaust-layout-needed-conversion.html)). Porsche calls it a "dual-flow exhaust system now incorporating two catalytic converters" ([Wikipedia, 993](https://en.wikipedia.org/wiki/Porsche_911_(993))). Two tailpipes, one per silencer ([Rose Passion parts diagram](https://www.rosepassion.com/en/diagrams-porsche-993-1998-eu-993-carrera-2-targa-automatic-gearbox/exhaust-106)). The 964 used a crossover and a single catalyst ([Pelican Parts thread](http://forums.pelicanparts.com/porsche-911-technical-forum/566890-993-exhaust-layout-needed-conversion.html)). So the per-bank 240-degree pattern is blended to an even 120-degree train before the silencers. Pipe diameters: unknown.

### Sound character

- 3rd-order source: 305 Hz at 6100 rpm, 40 Hz at 800 rpm idle ([Excellence](https://www.excellence-mag.com/issues/265/articles/porsche-engine-sounds)).
- Air cooling removes the water jacket's sound insulation, so valvetrain clatter and the fan are audible. Porsche enclosed the lower engine on the 964 to meet drive-by noise limits ([Excellence](https://www.excellence-mag.com/issues/265/articles/porsche-engine-sounds)). Car and Driver notes "the high-pitched screee of the engine fan" on the 993 ([Car and Driver](https://www.caranddriver.com/features/g17049524/cool-breeze-we-drive-historic-air-cooled-porsche-911s/)). Hagerty: the "mechanical chunter catches to give that trademark whirr from the cooling fan", then "an addictive flat-six howl" above 4000 rpm ([Hagerty UK](https://www.hagerty.co.uk/articles/buying-guide-1995-98-porsche-911-993/)).
- evo on a 993: "bassy, hollow dubba dubba dubba" at low speed, "so deep-chested you'd swear there are more than six cylinders", and "more metallic as it gulps in air" at the top of the range ([evo](https://www.evo.co.uk/porsche/911-carrera/7617/porsche-911-carrera-2)). The "dubba" rhythm is the per-bank 1.5-order pulse heard through the heat exchangers before the mixing chamber smooths it.
- Car and Driver: "very muffled engine note" inside a stock 993 ([Car and Driver](https://www.caranddriver.com/features/g17049524/cool-breeze-we-drive-historic-air-cooled-porsche-911s/)). Model the stock car as quiet with a strong mechanical layer.
- Fan: the fan speed ratio to crank is a fixed multiple, so fan whine is a clean tone that tracks rpm. Use 1.8 times crank with 5 blades (a 9th-order tone and harmonics) unless a 993-specific ratio is found (open question).
- Overrun: no 993-specific source found. Motronic sequential injection cuts fuel on overrun, so expect few pops on a stock car. Unverified.

## 4. Cross-plane V8: Ford 5.0 HO, Fox-body Mustang GT and LX

Why this engine: the 1987 to 1993 5.0 HO is the archetypal cross-plane V8 burble of the era, with a true dual exhaust, an H-pipe crossover, and well-documented firing order, compression, cam, rev limiter, and idle speed. Alternatives considered: Chevrolet L98 350 (Corvette C4, firing order 1-8-4-3-6-5-7-2) and Mercedes M119 (500E). The Ford has the best public data and the strongest cultural tie to the game's era.

### Identity

| Item | Value | Source |
| --- | --- | --- |
| Engine | 5.0L HO (302 cu in), 1987 to 1993 Mustang GT and LX 5.0 | [Fox Body Archive](https://www.thefoxbodyarchive.com/specs/engine) |
| Layout | 90-degree V8, cross-plane crank, OHV, cast iron block and heads | [Hemmings](https://www.hemmings.com/stories/1987-93-ford-mustang-gt/) |
| Bore x stroke | 4.000 x 3.000 in (101.6 x 76.2 mm) | [Fox Body Archive](https://www.thefoxbodyarchive.com/specs/engine) |
| Displacement | 302 cu in, 4942 cc | [Fox Body Archive](https://www.thefoxbodyarchive.com/specs/engine) |
| Compression | 9.0:1 | [Fox Body Archive](https://www.thefoxbodyarchive.com/specs/engine), [Ford Racing instruction sheet](https://www.performanceparts.ford.com/download/instructionsheets/FordInstShtM-6007-F50.pdf) |
| Heads | E7TE truck castings, 1987 to 1995 | [Hemmings](https://www.hemmings.com/stories/1987-93-ford-mustang-gt/) |
| Valves | 1.74 in intake, 1.45 in exhaust, 2 per cylinder | [Ford Racing instruction sheet](https://www.performanceparts.ford.com/download/instructionsheets/FordInstShtM-6007-F50.pdf) |
| Camshaft | Hydraulic roller, 0.444 in lift, 276 degrees intake and 266 degrees exhaust advertised duration, 116 degree intake and 115 degree exhaust centrelines (1995 production cam, same lift as the 1985 to 1993 cam) | [Ford Racing instruction sheet](https://www.performanceparts.ford.com/download/instructionsheets/FordInstShtM-6007-F50.pdf), [Ford V-8 Engine Workshop](https://www.wrljet.com/fordv8/50.html) |
| Induction | Sequential EFI, speed density 1986 to 1988, 55 mm mass air from 1989 (California 1988), 60 mm throttle body | [Ford V-8 Engine Workshop](https://www.wrljet.com/fordv8/50.html) |
| Balance | 50 oz-in external balance | [Ford Racing instruction sheet](https://www.performanceparts.ford.com/download/instructionsheets/FordInstShtM-6007-F50.pdf) |

### Firing order and intervals

1-3-7-2-6-5-4-8 ([Fox Body Archive](https://www.thefoxbodyarchive.com/specs/engine), [Ford Racing instruction sheet](https://www.performanceparts.ford.com/download/instructionsheets/FordInstShtM-6007-F50.pdf)). Ford numbers 1 to 4 on the passenger (right) bank front to back and 5 to 8 on the driver (left) bank ([Summit Racing](https://help.summitracing.com/knowledgebase/article/HE-04975/en-us)). This is the 351W order, the same pattern as the later GM LS ([hogervorst.tech](http://www.hogervorst.tech/craig/v8-engines.html)).

Bank sequence: R R L R L L R L. Right bank fires at 0, 90, 270, 540 degrees, so intervals 90-180-270-180. Left bank fires at 180, 360, 450, 630 degrees, so intervals 180-90-180-270. The whole engine is even at 90 degrees ([Wikipedia, Crossplane](https://en.wikipedia.org/wiki/Crossplane)).

### Idle, power, torque, redline

| Item | Value | Source |
| --- | --- | --- |
| Peak power | 225 hp at 4200 rpm (1987 to 1992); 205 hp (1993, new rating method) | [Fox Body Archive](https://www.thefoxbodyarchive.com/specs/engine), [Hemmings](https://www.hemmings.com/stories/1987-93-ford-mustang-gt/) |
| Power rpm conflict | 225 hp at 4000 rpm | [Ford V-8 Engine Workshop](https://www.wrljet.com/fordv8/50.html) |
| Peak torque | 300 lb-ft at 3200 rpm | [Fox Body Archive](https://www.thefoxbodyarchive.com/specs/engine) |
| Rev limiter | 6250 rpm fuel cut in every stock Fox EEC-IV calibration; it cuts one injector at a time | [Corral.net](https://www.corral.net/threads/is-there-a-factory-rev-limiter-on-89-93.709702/), [StangNet](https://stangnet.com/mustang-forums/threads/do-87-93s-have-govener-speed-limits.480211/) |
| Redline | Unverified. Stock valve springs float near the limiter | [Corral.net](https://www.corral.net/threads/is-there-a-factory-rev-limiter-on-89-93.709702/) |
| Idle | 675 rpm (M/T), 625 rpm (A/T), 1993 service data; ISC target 672 to 850 rpm | [Diagnostdata 1993 adjustments](https://diagnostdata.com/ford/mustang/iv-1993-1998/remont/adjustments/engine-controls-adjustments-50l/adjustments/), [StangNet idle thread](https://stangnet.com/mustang-forums/threads/ideal-idle-rpm.805964/) |
| Dyno reference | Stock-ish 5.0 with shorty headers: 221 rwhp, 296.5 lb-ft at the wheels | [FordMuscle](https://www.fordmuscle.com/tech-stories/exhausting-rehab-upgrading-our-fox-exhaust-with-bbk-and-flowmaster/) |

### Exhaust

Stainless tubular shorty headers from 1985, 2.25 in header outlets from 1986 ([Ford V-8 Engine Workshop](https://www.wrljet.com/fordv8/50.html)). "A true catalytic dual exhaust" with four catalysts: two light-off cats at the headers and two more at the H-pipe crossover, then two low-restriction silencers ([MotorTrend](https://www.motortrend.com/how-to/mump-1111-how-to-replace-a-fox-body-exhaust-system)). Stock pipe diameter 2.25 in, dual exit, silencers ahead of the axle ([AmericanMuscle](https://www.americanmuscle.com/understanding-mustang-exhaust-systems.html)). Two tailpipes. The H-pipe lets the two consecutive same-bank pulses scavenge into the empty opposite pipe and raises pitch slightly ([decarreteres](https://decarreteres.wordpress.com/2019/10/22/engine-technology-exhaust-noise/)). Aftermarket shorty primaries are 1.625 in; stock primary diameter is unverified ([MotorTrend](https://www.motortrend.com/how-to/1406-long-tube-vs-shorty-headers-pipe-dreams)).

Measured levels on a Fox with a stock-type system: 82 dB at idle, 95 dB cruising at 70 mph, 96 dB wide open, in-car ([FordMuscle](https://www.fordmuscle.com/tech-stories/exhausting-rehab-upgrading-our-fox-exhaust-with-bbk-and-flowmaster/)).

### Sound character

- Whole-engine 4th order: 280 Hz at 4200 rpm, 45 Hz at 675 rpm idle. Each bank sees 180-270-180-90, so each pipe carries 0.5-order and 1.5-order content under the 4th order. That is the burble ([Wikipedia, Crossplane](https://en.wikipedia.org/wiki/Crossplane), [howengineswork](https://www.howengineswork.com/engines/piston/sound)).
- The 4-into-1 bank manifold accentuates the pattern, "sometimes described as potato-potato" ([Wikipedia, Crossplane](https://en.wikipedia.org/wiki/Crossplane)). The two consecutive same-bank pulses create a pressure peak heard at the tailpipe ([hogervorst.tech](http://www.hogervorst.tech/craig/v8-engines.html)). A 1999 SAE paper finds the junction of two front pipes on a V8 adds first-order pressure content and worsens the sound when the junction has volume ([SAE 1999-01-1651](https://doi.org/10.4271/1999-01-1651)).
- The H-pipe crossover is the signature of the Fox sound; owners warn that removing it loses the character ([Reddit r/foxmustang](https://www.reddit.com/r/foxmustang/comments/8kazk9/exhaust_setup_suggestions_for_a_bone_stock_50_lx/)).
- Overrun: EEC-IV has deceleration fuel shut-off, and the four catalysts suppress pops. Stock cars burble and gurgle on lift-off but rarely crack. Batch-fire or no-fuel-cut cars pop far more ([PistonHeads Speed Six thread](https://www.pistonheads.com/gassing/topic.asp?f=152&h=0&t=1069481)). Unverified for this specific car.

## Overrun pops and crackles: mechanism

Pops come from fuel burning in the exhaust instead of the cylinder. Causes, in order of effect:

1. Fuel still injected on lift-off, with late ignition, so combustion is still in progress when the exhaust valve opens ([StrikeEngine on Ford's factory pop calibration](https://strikeengine.com/exhaust-burbelspopscrackles-how-its-done/), [Thanas tuning](https://thanas.co.za/blog/25-tuning-for-exhaust-crackles/body)).
2. Valve overlap pushing fresh mixture into the exhaust, ignited by the next hot pulse. Larger with long-duration cams ([PistonHeads](https://www.pistonheads.com/gassing/topic.asp?f=13&h=0&t=425533)).
3. Batch-fire or continuous injection (K-Jetronic) leaving fuel in the port on overrun ([PistonHeads Speed Six thread](https://www.pistonheads.com/gassing/topic.asp?f=152&h=0&t=1069481)).

Cars with deceleration fuel cut (modern sequential EFI) pop little, and catalysts absorb the rest. For the four profiles, rank expected stock overrun activity: Testarossa decat (most) > Testarossa cat > Fox 5.0 > 993 > Type R (least). This ranking is an inference, not a measurement.

## Reference recordings

Freesound licence text "You can copy, modify, distribute and perform the sound, even for commercial purposes, all without the need of asking permission" is CC0. The "sample, mash-up ... noncommercial" text is the Sampling+ licence.

| Engine | Recording | Licence | Notes |
| --- | --- | --- | --- |
| Air-cooled 911 | [Porsche idle.aif by victormalloy](https://www.freesound.org/people/victormalloy/sounds/113216/) | CC0 | 96 kHz, 24 bit, stereo, 17 s idle. Model not stated |
| Air-cooled 911 | [Porsche Exhaust.aif, same pack](https://freesound.org/people/victormalloy/packs/7118/) | Pack is CC0 per the idle entry; check each file | 96 kHz recordings of luxury sports cars |
| Air-cooled 911 | [porsche001.wav by djbono](https://freesound.org/people/djbono/sounds/18773/) | Sampling+ (non-commercial distribution) | 1989 911 rev from 600 to 5200 rpm, from behind. Internal listening only |
| Ford V8 | [Ford Mustang Engine (1985).wav by lmbubec](https://freesound.org/s/119449/) | CC0 | 1985 Mustang idle and shutdown, 48 kHz, 24 bit. 1985 is a carburetted 5.0 HO, same block and crank |
| Ford V8 | [Mustang Ehxaust.aif, victormalloy pack](https://freesound.org/people/victormalloy/packs/7118/) | Check file | 96 kHz |
| Honda inline-4 | [Honda Engine Start and Rev.wav by thepodcastdoctor](https://freesound.org/people/thepodcastdoctor/sounds/487553/) | CC0 | 2012 Civic (R18, SOHC). Only useful for inline-4 order structure, not B18C character |
| Honda B16A | [Crankcase Audio REV, Acura Integra 1992](https://www.audiokinetic.com/en/products/wwise-add-ons/crankcase-rev-pricing/) | Commercial, paid | Closest available B-series VTEC capture, for reference only |
| Ferrari flat-12 | No free-licence recording found | | YouTube onboard clips exist, for example [Robbert Alblas onboard](https://www.youtube.com/watch?v=mvwNXNJqg3U). Not redistributable. Most are decat or aftermarket |

## Implications for snarl

1. The per-bank exhaust model is the main differentiator. Four distinct per-bank interval patterns fall out of the data: 120 even (flat-12), 180 even single bank (inline-4), 240 even (flat-6), and 90-180-270-180 (V8). If the engine model generates per-bank pulse trains and a crossover or merge stage, all four characters follow without special cases.
2. Treat crossover as a profile parameter with three states: none (Testarossa, pending the silencer question), full merge before the silencers (993 mixing chamber), partial (Fox H-pipe). The H-pipe must leave most of the 0.5-order content in place or the V8 loses its burble.
3. Dominant order and idle fundamental per profile: flat-12 6th order (50 Hz per bank at 1000 rpm), inline-4 2nd order (26.7 Hz at 800), flat-6 3rd order (40 Hz at 800), V8 4th order (45 Hz at 675). The V8 and flat-6 need the most low-frequency headroom at idle.
4. Rev range sets pitch span. The Type R runs to 8500 rpm fuel cut, the Testarossa and 993 to 6700 to 6800, the Fox to 6250. The flat-12 reaches 680 Hz at the 6th order, the Type R 283 Hz at the 2nd order, so the Testarossa is the highest-pitched profile despite the lower rpm.
5. Limiter behaviour differs: the Fox cuts one injector at a time (soft stutter), the Type R has a hard fuel cut at 8500, the 993 is governed at 6700 +/- 20. The Testarossa limiter is unknown, so model a soft ignition cut until verified.
6. Mechanical layer: only the 993 needs a fan whine tone and valvetrain clatter. The Testarossa needs belt and 48-valve bucket tappet noise at low level. The Type R needs an induction roar that steps up at VTEC changeover (5700 rpm US).
7. Overrun: make pop density a per-profile parameter driven by injection type. Suggested starting points: Testarossa 0.4 (0.8 for a decat variant), Fox 0.25, 993 0.1, Type R 0.05. These are inferences for tuning, not measurements.
8. Power curves: use the peak points above plus the review comments on where each engine wakes up (Testarossa and 993 at about 5000 rpm, Type R at 5700 rpm VTEC, Fox torque peak at 3200 rpm) to shape load-dependent timbre.

## Open questions

1. Testarossa firing order for F113. The 512 BB value is well supported, and the architecture is shared, but the Testarossa manual was not read. Fetching the ManualsLib technical manual "Timing Data" pages would settle it along with valve timing and idle speed.
2. Testarossa silencer internals. The parts table shows one silencer fed by two bank pipes with four tailpipes. Whether the bank flows mix inside decides the crossover state for the flat-12.
3. Testarossa rev limiter rpm and type (ignition cut by Microplex is likely).
4. Pipe diameters and lengths for the Testarossa, Type R, and 993 systems. Only the Fox has a documented 2.25 in pipe size.
5. 993 fan to crank ratio and blade count. The 1.8 ratio and 5 blades are for the 1976 930 fan, not confirmed for the 993.
6. 993 and 964 valve timing values are from poor OCR. Verify against a clean scan of the Porsche booklet.
7. Fox 5.0 redline marking on the factory tachometer, and stock shorty header primary diameter.
8. A free-licence recording of any Ferrari flat-12 and of a B-series VTEC. None was found. Internal listening can use YouTube, but nothing from there can ship.
9. JDM B18C rev limit: 8900 (Wikipedia, uncited) versus 8500 (integratyper.org). The US 8500 figure is factory.
