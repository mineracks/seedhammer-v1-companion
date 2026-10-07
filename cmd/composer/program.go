//go:build js && wasm

package main

// Program-level exports: the bridge Any Two Keys (and any other host with a serial port)
// uses. Everything here goes through plate.Build, the same rasteriser the Pi runs, so the
// strokes a host streams are the strokes the Pi would have produced for the same SH1E.
//
//	composerBuildDesign(json)       -> Uint8Array (SH1E)   generic: text blocks, svg paths, qr blocks
//	composerShareDesign(json)       -> {side_a, side_b}    one Shamir share, two SH1E designs
//	composerProgram(sh1eBytes)      -> {commands, ...}     10-byte engraver commands in PLATE steps
//	composerPreview(sh1eBytes)      -> string (SVG)        what the strokes look like on the plate
//
// Commands are plate-relative (origin = plate top-left, 126 steps/mm). The host adds the
// jig offset for the plate type (SH-02 Square: 97,49 mm; SH-03 Large: 97,0 mm from home)
// and runs the mjolnir lifecycle - see cmd/hammer and docs/architecture/v1-engrave-spec.md.

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/mineracks/seedhammer-v1-companion/engrave"
	"github.com/mineracks/seedhammer-v1-companion/engrave/wire/sh1e"
	"github.com/mineracks/seedhammer-v1-companion/plate"
)

// A Go panic inside a js.FuncOf callback takes the whole WASM program down, after which every
// export returns undefined (seen in Any Two Keys on 2026-10-07 as "undefined is not an object
// (evaluating 'd.side_a')"). So these exports never panic: an error comes back as {error: "..."}.
func fail(err error) any { return js.ValueOf(map[string]any{"error": err.Error()}) }

func guarded(f func(js.Value, []js.Value) any) func(js.Value, []js.Value) any {
	return func(this js.Value, args []js.Value) (out any) {
		defer func() {
			if r := recover(); r != nil {
				out = fail(fmt.Errorf("engine: %v", r))
			}
		}()
		return f(this, args)
	}
}

func init() {
	js.Global().Set("composerBuildDesign", js.FuncOf(guarded(exportBuildDesign)))
	js.Global().Set("composerShareDesign", js.FuncOf(guarded(exportShareDesign)))
	js.Global().Set("composerProgram", js.FuncOf(guarded(exportProgram)))
	js.Global().Set("composerPreview", js.FuncOf(guarded(exportPreview)))
}

type designJSON struct {
	Plate      int `json:"plate"`
	TextBlocks []struct {
		Size  int    `json:"size_pt"`
		X     int    `json:"x_mm"`
		Y     int    `json:"y_mm"`
		Align int    `json:"align"`
		Text  string `json:"text"`
		Rot   int    `json:"rotation"`
	} `json:"text_blocks"`
	SvgPaths []struct {
		X     int    `json:"x_mm"`
		Y     int    `json:"y_mm"`
		Scale int    `json:"scale_pct"`
		D     string `json:"d"`
		Rot   int    `json:"rotation"`
	} `json:"svg_paths"`
	QRBlocks []struct {
		X      int    `json:"x_mm"`
		Y      int    `json:"y_mm"`
		Module int    `json:"module_tenths"`
		Level  int    `json:"level"`
		Data   []byte `json:"data"` // base64 in JSON
	} `json:"qr_blocks"`
}

func exportBuildDesign(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return fail(fmt.Errorf("expected (json)"))
	}
	var in designJSON
	if err := json.Unmarshal([]byte(args[0].String()), &in); err != nil {
		return fail(fmt.Errorf("design json: %w", err))
	}
	d := sh1e.Design{PlateType: sh1e.PlateType(in.Plate)}
	for _, t := range in.TextBlocks {
		d.TextBlocks = append(d.TextBlocks, sh1e.TextBlock{FontID: sh1e.FontConstant, Size: uint16(t.Size), XMM: int16(t.X), YMM: int16(t.Y), Alignment: sh1e.Alignment(t.Align), Text: t.Text, Rotation: uint16(t.Rot)})
	}
	for _, p := range in.SvgPaths {
		sc := p.Scale
		if sc == 0 {
			sc = 100
		}
		d.SvgPaths = append(d.SvgPaths, sh1e.SvgPath{XMM: int16(p.X), YMM: int16(p.Y), ScalePct: uint16(sc), PathD: p.D, Rotation: uint16(p.Rot)})
	}
	for _, q := range in.QRBlocks {
		m := q.Module
		if m == 0 {
			m = 9
		}
		d.QRBlocks = append(d.QRBlocks, sh1e.QRBlock{XMM: int16(q.X), YMM: int16(q.Y), ModuleTenths: uint8(m), Level: uint8(q.Level), Data: q.Data})
	}
	b, err := sh1e.Encode(d)
	if err != nil {
		return fail(err)
	}
	return uint8Array(b)
}

type shareJSON struct {
	Title       string `json:"title"`
	UrType      string `json:"ur_type"`
	Fingerprint string `json:"fingerprint"`
	K           int    `json:"k"`
	N           int    `json:"n"`
	Index       int    `json:"index"`
	Payload     []byte `json:"payload"` // base64 in JSON
}

func exportShareDesign(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return fail(fmt.Errorf("expected (json)"))
	}
	var in shareJSON
	if err := json.Unmarshal([]byte(args[0].String()), &in); err != nil {
		return fail(fmt.Errorf("share json: %w", err))
	}
	a, b, err := plate.ShareDesign(plate.Share{Title: in.Title, UrType: in.UrType, Fingerprint: in.Fingerprint, K: in.K, N: in.N, Index: in.Index, Payload: in.Payload})
	if err != nil {
		return fail(err)
	}
	ea, err := sh1e.Encode(a)
	if err != nil {
		return fail(err)
	}
	eb, err := sh1e.Encode(b)
	if err != nil {
		return fail(err)
	}
	return js.ValueOf(map[string]any{"side_a": uint8Array(ea), "side_b": uint8Array(eb), "text": plate.ShareText(in.Payload)})
}

func decodeArg(args []js.Value) (plate.Result, error) {
	if len(args) != 1 {
		return plate.Result{}, fmt.Errorf("expected (sh1eBytes)")
	}
	raw := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(raw, args[0])
	d, err := sh1e.Decode(raw)
	if err != nil {
		return plate.Result{}, err
	}
	return plate.Build(d)
}

func exportProgram(this js.Value, args []js.Value) any {
	r, err := decodeArg(args)
	if err != nil {
		return fail(err)
	}
	buf := make([]byte, 0, r.Commands*10)
	r.Plan(func(c engrave.Command) {
		op := byte(0x80)
		if c.Line {
			op = 0x00
		}
		x, y := c.Coord.X, c.Coord.Y
		if x < 0 || y < 0 || x > 0xffffff || y > 0xffffff {
			return
		}
		buf = append(buf, op, byte(x), byte(x>>8), byte(x>>16), byte(y), byte(y>>8), byte(y>>16), 0, 0, 0)
	})
	warn := make([]any, 0, len(r.Warnings))
	for _, w := range r.Warnings {
		warn = append(warn, w)
	}
	mm := float64(plate.Params.Millimeter)
	return js.ValueOf(map[string]any{
		"commands":  uint8Array(buf),
		"count":     len(buf) / 10,
		"strokes":   r.Strokes,
		"plate":     int(r.Plate),
		"width_mm":  r.DimsMM.X,
		"height_mm": r.DimsMM.Y,
		"bounds_mm": map[string]any{"x0": float64(r.Bounds.Min.X) / mm, "y0": float64(r.Bounds.Min.Y) / mm, "x1": float64(r.Bounds.Max.X) / mm, "y1": float64(r.Bounds.Max.Y) / mm},
		"warnings":  warn,
	})
}

func exportPreview(this js.Value, args []js.Value) any {
	r, err := decodeArg(args)
	if err != nil {
		return fail(err)
	}
	return plate.Preview(r)
}
