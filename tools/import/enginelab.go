package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type elFile struct {
	Engine elEngine `json:"engine"`
}

type elEngine struct {
	Layout        string  `json:"layout"`
	Cycle         string  `json:"cycle"`
	CylinderCount int     `json:"cylinder_count"`
	FiringOrder   []int   `json:"firing_order"`
	IdleRPM       float64 `json:"idle_rpm"`
	RedlineRPM    float64 `json:"redline_rpm"`
	BankAngleDeg  float64 `json:"bank_angle_deg"`
	Ignition      struct {
		RevLimitRPM *float64 `json:"rev_limit_rpm"`
		LimiterS    *float64 `json:"limiter_duration_s"`
		KeepsFuel   bool     `json:"limiter_keeps_fuel"`
	} `json:"ignition"`
	Uses struct {
		Exhaust string `json:"exhaust"`
	} `json:"uses"`
	Exhaust      map[string]any `json:"exhaust"`
	ExhaustPaths []elPath       `json:"exhaust_paths"`
	Cylinders    []elCylinder   `json:"cylinders"`
}

type elCylinder struct {
	ID               int      `json:"id"`
	CrankOffsetDeg   *float64 `json:"crank_offset_deg"`
	IgnitionOffset   float64  `json:"ignition_offset_deg"`
	ExhaustPrimaryMM *float64 `json:"exhaust_primary_length_mm"`
}

type elPath struct {
	CylinderIDs []int          `json:"cylinder_ids"`
	AudioVolume *float64       `json:"audio_volume"`
	Geometry    map[string]any `json:"geometry"`
	Graph       *elGraph       `json:"graph"`
}

type elGraph struct {
	Components []elComponent `json:"components"`
	Cylinders  []struct {
		CylinderID int `json:"cylinder_id"`
		To         int `json:"to_component_id"`
	} `json:"cylinder_connections"`
	Connections []struct {
		From     int  `json:"from_component_id"`
		To       int  `json:"to_component_id"`
		FromPort *int `json:"from_port"`
		ToPort   *int `json:"to_port"`
	} `json:"connections"`
}

type elComponent struct {
	ID        int     `json:"id"`
	Type      string  `json:"type"`
	LengthMM  float64 `json:"length_mm"`
	DiameterM float64 `json:"diameter_mm"`
	VolumeL   float64 `json:"volume_l"`
	Coupling  float64 `json:"crossover_coupling"`
}

// elGeometry is the scalar exhaust of EngineLab, ExhaustConfig in
// src/foundation/include/enginelab/foundation/EngineTypes.hpp, in mm.
type elGeometry struct {
	PrimaryLength, CollectorDiameter, OutletDiameter float64
	ChamberDiameter, ChamberLength, Midpipe          float64
}

// elDefaultGeometry is the ExhaustConfig default.
var elDefaultGeometry = elGeometry{PrimaryLength: 480, CollectorDiameter: 58, OutletDiameter: 65}

// elNames names the EngineLab engines. The identifiers and descriptions
// name no maker or model.
var elNames = map[string]naming{
	"01_honda_k20a_like.engine.yaml":            {"el-i4-dohc-cam-switch", "Inline-4, DOHC with a cam profile switch, 4-2-1 style header"},
	"02_toyota_2jz_gte_like.engine.yaml":        {"el-i6-30-turbo", "Inline-6, 3.0 litre turbo"},
	"03_gm_ls3_like.engine.yaml":                {"el-v8-crossplane-pushrod", "Cross-plane V8, pushrod, X-pipe, two silencers"},
	"04_subaru_ej25_like.engine.yaml":           {"el-f4-boxer-turbo", "Flat-4 boxer turbo"},
	"05_audi_i5_like.engine.yaml":               {"el-i5-turbo", "Inline-5 turbo"},
	"06_hayabusa_i4_like.engine.yaml":           {"el-i4-moto-litre", "Inline-4 motorcycle engine, 180 degree crank"},
	"07_harley_v_twin_like.engine.yaml":         {"el-v2-45-big-twin", "45 degree V-twin on a shared crank pin, uneven firing"},
	"08_merlin_v12_like.engine.yaml":            {"el-v12-60-aero", "60 degree V12 aero engine, short stacks"},
	"09_porsche_flat6_like.engine.yaml":         {"el-f6-aircooled", "Air-cooled flat-6, two 3-into-1 headers"},
	"10_radial5_like.engine.yaml":               {"el-r5-radial", "5-cylinder radial aero engine"},
	"11_yamaha_cp2_mt07_like.engine.yaml":       {"el-i2-270", "Parallel twin, 270 degree crank"},
	"12_yamaha_cp3_mt09_like.engine.yaml":       {"el-i3-240", "Inline-3 motorcycle engine, 240 degree crank"},
	"13_yamaha_cp4_mt10_like.engine.yaml":       {"el-i4-crossplane-moto", "Cross-plane inline-4 motorcycle engine, uneven firing"},
	"14_vw_2_0_tdi_like.engine.yaml":            {"el-i4-diesel-turbo", "Inline-4 turbo diesel"},
	"15_cp2_full_system_like.engine.yaml":       {"el-i2-270-full-system", "Parallel twin, 270 degree crank, 2-into-1 race system"},
	"16_audio_physics_lab_689_twin.engine.yaml": {"el-i2-270-lab", "Parallel twin, 270 degree crank, packed silencer test engine"},
	"17_yamaha_rd350lc_like.engine.yaml":        {"el-i2-two-stroke", "Two-stroke parallel twin, expansion chambers"},
}

// loadEngineLab reads dir/engines/*.json and the exhaust presets in
// dir/exhausts.json.
func loadEngineLab(dir string) ([]engine, error) {
	data, err := os.ReadFile(filepath.Join(dir, "exhausts.json"))
	if err != nil {
		return nil, err
	}
	var presets map[string]map[string]any
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, fmt.Errorf("exhausts.json: %w", err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "engines", "*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	var out []engine
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var d elFile
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		rel := strings.TrimSuffix(filepath.Base(f), ".json") + ".engine.yaml"
		e, err := fromEngineLab(rel, &d.Engine, presets)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		out = append(out, e)
	}
	return out, nil
}

func fromEngineLab(rel string, d *elEngine, presets map[string]map[string]any) (engine, error) {
	n := elNames[rel]
	e := engine{
		ID: n.id, Project: "EngineLab", File: rel, Description: n.description,
		IdleRPM: d.IdleRPM, RedlineRPM: d.RedlineRPM, LimiterRPM: d.RedlineRPM, LimiterMS: 80,
		LimiterKind: "fuel", Crossover: "none",
	}
	if v := d.Ignition.RevLimitRPM; v != nil {
		e.LimiterRPM = *v
	}
	if v := d.Ignition.LimiterS; v != nil {
		e.LimiterMS = min(500, 1000**v)
	}
	if d.Ignition.KeepsFuel {
		e.LimiterKind = "ignition"
		e.Notes = append(e.Notes, "The limiter keeps the fuel, so limiter_kind is ignition.")
	}

	// The crank offsets, as EngineLab sets them: explicit per cylinder, or
	// even over the cycle in firing order (applyCrankOffsets in
	// src/catalog/src/EngineCatalog.cpp).
	cycle := 720.0
	if d.Cycle == "two_stroke" {
		cycle = 360
	}
	cyls := d.Cylinders
	if len(cyls) == 0 {
		for id := 1; id <= d.CylinderCount; id++ {
			cyls = append(cyls, elCylinder{ID: id})
		}
	}
	offsets := map[int]float64{}
	for _, c := range cyls {
		switch {
		case c.CrankOffsetDeg != nil:
			offsets[c.ID] = *c.CrankOffsetDeg
		case len(d.Cylinders) == 0:
			if k := slices.Index(d.FiringOrder, c.ID); k >= 0 {
				offsets[c.ID] = float64(k) * cycle / float64(len(d.FiringOrder))
			}
		}
		offsets[c.ID] += c.IgnitionOffset
	}

	geom := presetGeometry(elDefaultGeometry, presets[d.Uses.Exhaust])
	if d.Exhaust != nil {
		geom = presetGeometry(elDefaultGeometry, d.Exhaust)
	}
	groups, err := elGroups(d, cyls, geom)
	if err != nil {
		return engine{}, err
	}
	e.Crossover, e.Coupling = groups.crossover, groups.coupling
	if len(groups.paths) > 4 {
		return engine{}, fmt.Errorf("%d exhaust banks, snarl takes 4", len(groups.paths))
	}
	e.Paths = groups.paths

	first := offsets[cyls[0].ID]
	for _, c := range cyls {
		deg := offsets[c.ID] - first
		primary := groups.primary[c.ID]
		if c.ExhaustPrimaryMM != nil {
			primary = *c.ExhaustPrimaryMM / 1000
		}
		e.Cylinders = append(e.Cylinders, cylinder{CrankDeg: wrap720(deg), Bank: groups.bank[c.ID], PrimaryM: primary})
		if cycle == 360 {
			e.Cylinders = append(e.Cylinders, cylinder{CrankDeg: wrap720(deg + 360), Bank: groups.bank[c.ID], PrimaryM: primary})
		}
	}
	if cycle == 360 {
		e.Notes = append(e.Notes, "Two-stroke: each cylinder fires every 360 degrees, so each is two snarl cylinders 360 degrees apart.")
	}
	e.Layout = elLayout(d, len(cyls))
	return e, nil
}

func elLayout(d *elEngine, n int) string {
	switch {
	case n == 1:
		return "single"
	case d.Layout == "v":
		return fmt.Sprintf("V%d, %.0f degree", n, d.BankAngleDeg)
	case d.Layout == "flat":
		return fmt.Sprintf("flat-%d", n)
	case d.Layout == "radial":
		return fmt.Sprintf("radial-%d", n)
	}
	return fmt.Sprintf("inline-%d", n)
}

func number(m map[string]any, key string) (float64, bool) {
	v, ok := m[key].(float64)
	return v, ok
}

// presetGeometry applies the keys of an exhaust preset over g.
func presetGeometry(g elGeometry, m map[string]any) elGeometry {
	for key, dst := range map[string]*float64{
		"primary_length_mm":           &g.PrimaryLength,
		"collector_diameter_mm":       &g.CollectorDiameter,
		"outlet_diameter_mm":          &g.OutletDiameter,
		"muffler_chamber_diameter_mm": &g.ChamberDiameter,
		"muffler_chamber_length_mm":   &g.ChamberLength,
		"midpipe_length_mm":           &g.Midpipe,
	} {
		if v, ok := number(m, key); ok {
			*dst = v
		}
	}
	return g
}

// scalarPath maps a scalar exhaust to a snarl path. A path with no mid pipe
// gets a collector of 0.2 m, the shortest that the import uses.
func scalarPath(g elGeometry, gain float64) exhaustPath {
	p := exhaustPath{
		CollectorM:    max(0.2, g.Midpipe/1000),
		PipeRadiusM:   g.CollectorDiameter / 2000,
		OutletRadiusM: g.OutletDiameter / 2000,
		Gain:          gain,
	}
	if g.ChamberDiameter > 0 && g.ChamberLength > 0 && g.ChamberDiameter > g.CollectorDiameter {
		p.ChamberM = g.ChamberLength / 1000
		ratio := g.ChamberDiameter / g.CollectorDiameter
		p.ChamberArea = round(ratio*ratio, 2)
	}
	return p
}

// elBanks is the exhaust grouping of an EngineLab engine in snarl terms.
type elBanks struct {
	bank      map[int]int     // cylinder id to bank
	primary   map[int]float64 // cylinder id to primary length in metres
	paths     []exhaustPath
	crossover string
	coupling  float64
}

// elGroups groups the cylinders into snarl banks. Without a component
// graph, each exhaust path is a bank. With a graph, the cylinders that
// meet at the same first junction are a bank.
func elGroups(d *elEngine, cyls []elCylinder, geom elGeometry) (elBanks, error) {
	g := elBanks{bank: map[int]int{}, primary: map[int]float64{}, crossover: "none"}
	paths := d.ExhaustPaths
	if len(paths) == 0 {
		var ids []int
		for _, c := range cyls {
			ids = append(ids, c.ID)
		}
		paths = []elPath{{CylinderIDs: ids}}
	}
	loudest := 0.0
	for _, p := range paths {
		loudest = max(loudest, volume(p))
	}
	for _, p := range paths {
		if p.Graph != nil {
			if err := g.addGraph(p, volume(p)/loudest); err != nil {
				return g, err
			}
			continue
		}
		pg := geom
		if p.Geometry != nil {
			pg = presetGeometry(elDefaultGeometry, p.Geometry)
		}
		for _, id := range p.CylinderIDs {
			g.bank[id] = len(g.paths)
			g.primary[id] = pg.PrimaryLength / 1000
		}
		g.paths = append(g.paths, scalarPath(pg, volume(p)/loudest))
	}
	for _, c := range cyls {
		if _, ok := g.bank[c.ID]; !ok {
			return g, fmt.Errorf("cylinder %d is on no exhaust path", c.ID)
		}
	}
	return g, nil
}

func volume(p elPath) float64 {
	if p.AudioVolume != nil {
		return *p.AudioVolume
	}
	return 1
}

// addGraph adds the banks of one component graph. From each cylinder, the
// pipes up to the first junction are its primary. From each junction, the
// components up to the outlet are the collector, with a silencer as the
// chamber. A crossover component gives a partial crossover. When two banks
// reach the same merge, the crossover is full.
func (g *elBanks) addGraph(p elPath, gain float64) error {
	gr := p.Graph
	comp := map[int]elComponent{}
	for _, c := range gr.Components {
		comp[c.ID] = c
	}
	inbound := map[int]int{}
	for _, c := range gr.Connections {
		inbound[c.To]++
	}
	next := func(id int, port *int) (int, *int, bool) {
		for _, c := range gr.Connections {
			if c.From == id && (port == nil || c.FromPort == nil || *c.FromPort == *port) {
				return c.To, c.ToPort, true
			}
		}
		return 0, nil, false
	}
	type start struct{ id int }
	var junctions []start
	visited := map[int]int{} // merge component id to the bank that reached it first
	for _, cc := range gr.Cylinders {
		id, length := cc.To, 0.0
		var port *int
		single := len(p.CylinderIDs) == 1
		for {
			c, ok := comp[id]
			if !ok {
				return fmt.Errorf("cylinder %d reaches unknown component %d", cc.CylinderID, id)
			}
			if inbound[id] >= 2 || c.Type != "pipe" {
				break
			}
			length += c.LengthMM / 1000
			n, np, ok := next(id, port)
			if !ok || single {
				// A single-cylinder path has no junction: its first pipe is
				// the primary and the rest is the collector.
				id, port = n, np
				if !ok {
					return fmt.Errorf("cylinder %d reaches no outlet", cc.CylinderID)
				}
				break
			}
			id, port = n, np
		}
		k := slices.IndexFunc(junctions, func(s start) bool { return s.id == id })
		if k < 0 {
			k = len(junctions)
			junctions = append(junctions, start{id})
		}
		g.bank[cc.CylinderID] = len(g.paths) + k
		g.primary[cc.CylinderID] = length
	}
	base := len(g.paths)
	for k, j := range junctions {
		path := exhaustPath{Gain: gain}
		id := j.id
		var port *int
		for range len(gr.Components) + 1 {
			c := comp[id]
			switch c.Type {
			case "muffler":
				path.ChamberM = c.LengthMM / 1000
				if path.PipeRadiusM > 0 && c.LengthMM > 0 {
					area := c.VolumeL / 1000 / (c.LengthMM / 1000)
					path.ChamberArea = round(area/(math.Pi*path.PipeRadiusM*path.PipeRadiusM), 2)
				}
			case "outlet":
				path.OutletRadiusM = c.DiameterM / 2000
				path.CollectorM += c.LengthMM / 1000
			case "crossover":
				g.crossover, g.coupling = "partial", min(0.5, c.Coupling)
			default:
				if path.PipeRadiusM == 0 && c.DiameterM > 0 {
					path.PipeRadiusM = c.DiameterM / 2000
				}
				path.CollectorM += c.LengthMM / 1000
				if c.Type == "merge" && inbound[id] >= 2 {
					if b, ok := visited[id]; ok && b != base+k {
						g.crossover = "full"
					} else {
						visited[id] = base + k
					}
				}
			}
			n, np, ok := next(id, port)
			if !ok {
				break
			}
			id, port = n, np
		}
		if path.OutletRadiusM == 0 {
			path.OutletRadiusM = path.PipeRadiusM
		}
		if path.ChamberArea <= 1 {
			path.ChamberM, path.ChamberArea = 0, 0
		}
		path.CollectorM = max(0.2, path.CollectorM)
		g.paths = append(g.paths, path)
	}
	return nil
}

func round(x float64, places int) float64 {
	s := math.Pow(10, float64(places))
	return math.Round(x*s) / s
}
