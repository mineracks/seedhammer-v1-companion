// Package plate turns an SH1E design into engraver strokes. It is the ONE rasteriser:
// the composer (WASM, inside Any Two Keys or the browser) and the Pi firmware both call
// Build, so a preview and the plate that comes off the machine are the same strokes by
// code identity. Nothing here knows about serial ports or jigs; coordinates are plate
// steps (Params.Millimeter per mm) from the plate's top-left corner.
package plate

import (
	"errors"
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/kortschak/qr"

	"github.com/mineracks/seedhammer-v1-companion/engrave"
	"github.com/mineracks/seedhammer-v1-companion/engrave/wire/sh1e"
	"github.com/mineracks/seedhammer-v1-companion/font/constant"
	"github.com/mineracks/seedhammer-v1-companion/font/vector"
)

// Params are the v1 machine's: a 0.3 mm needle stroke, 126 steps per millimetre.
var Params = engrave.Params{StrokeWidth: 38, Millimeter: 126}

// Margins from upstream backup/backup.go. Nuts and bolts hold a plate at its edges, so
// nothing is engraved outside the outer margin; the Large plate also has holes mid-way,
// which upstream avoids by keeping content in the 85 mm band 24.5 mm down.
const (
	OuterMarginMM = 3
	InnerMarginMM = 10
	LargeBandTop  = 24.5
	LargeBandH    = 85
)

// Dims of each plate type in millimetres.
func Dims(t sh1e.PlateType) (image.Point, bool) {
	switch t {
	case sh1e.SmallPlate:
		return image.Pt(85, 55), true
	case sh1e.SquarePlate:
		return image.Pt(85, 85), true
	case sh1e.LargePlate:
		return image.Pt(85, 134), true
	}
	return image.Point{}, false
}

// Result is a rasterised design.
type Result struct {
	Plan     engrave.Plan
	Plate    sh1e.PlateType
	DimsMM   image.Point
	Bounds   image.Rectangle // steps, plate-relative, pen-down strokes only
	Strokes  int             // pen-down segments
	Commands int             // moves + lines
	Warnings []string
}

var ErrOutsidePlate = errors.New("plate: design reaches outside the engraveable area")

// Build rasterises d. It fails if any stroke would land outside the outer margin, and
// warns (but allows) strokes inside the inner margin or across the Large plate's holes.
func Build(d sh1e.Design) (Result, error) {
	dims, ok := Dims(d.PlateType)
	if !ok {
		return Result{}, fmt.Errorf("plate: unknown plate type %d", d.PlateType)
	}
	mm := Params.Millimeter
	var plans []engrave.Plan
	var warnings []string
	face := constant.Font
	for i, tb := range d.TextBlocks {
		if tb.FontID != sh1e.FontConstant {
			// Comfortaa and Poppins are the LCD's bitmap faces; the engraver has one
			// stroke font. Draw with it rather than refuse a design made on the phone.
			warnings = append(warnings, fmt.Sprintf("text block %d: font %d is not a stroke font, drawn with the engraver's", i, tb.FontID))
		}
		p, err := textBlock(face, tb)
		if err != nil {
			return Result{}, fmt.Errorf("text block %d: %w", i, err)
		}
		plans = append(plans, p)
	}
	for i, sp := range d.SvgPaths {
		p, err := svgPath(sp)
		if err != nil {
			return Result{}, fmt.Errorf("svg path %d: %w", i, err)
		}
		plans = append(plans, p)
	}
	for i, q := range d.QRBlocks {
		p, err := qrBlock(q)
		if err != nil {
			return Result{}, fmt.Errorf("qr block %d: %w", i, err)
		}
		plans = append(plans, p)
	}
	plan := engrave.Commands(plans...)
	b := engrave.Measure(plan)
	strokes, cmds := 0, 0
	plan(func(c engrave.Command) {
		cmds++
		if c.Line {
			strokes++
		}
	})
	outer := image.Rect(OuterMarginMM*mm, OuterMarginMM*mm, (dims.X-OuterMarginMM)*mm, (dims.Y-OuterMarginMM)*mm)
	if strokes > 0 && !b.In(outer) {
		return Result{}, fmt.Errorf("%w: strokes span (%.1f,%.1f)-(%.1f,%.1f) mm, engraveable (%d,%d)-(%d,%d) mm",
			ErrOutsidePlate, f(b.Min.X), f(b.Min.Y), f(b.Max.X), f(b.Max.Y),
			OuterMarginMM, OuterMarginMM, dims.X-OuterMarginMM, dims.Y-OuterMarginMM)
	}
	inner := image.Rect(InnerMarginMM*mm, InnerMarginMM*mm, (dims.X-InnerMarginMM)*mm, (dims.Y-InnerMarginMM)*mm)
	if strokes > 0 && !b.In(inner) {
		warnings = append(warnings, "strokes come within 10 mm of the plate edge, near the mounting bolts")
	}
	// The Large plate's middle bolts sit 5 mm in from both long edges at mid-height; the
	// inner margin already clears them, so a design inside it needs no further warning.
	// (LargeBandTop/LargeBandH describe where upstream parks a Square layout on a Large
	// plate; they are a layout hint, not a rule.)
	return Result{Plan: plan, Plate: d.PlateType, DimsMM: dims, Bounds: b, Strokes: strokes, Commands: cmds, Warnings: warnings}, nil
}

func f(steps int) float64 { return float64(steps) / float64(Params.Millimeter) }

// ---- text ------------------------------------------------------------------------------

// textBlock lays a block out with its top-left (or top-centre / top-right, per alignment)
// at (XMM, YMM). Size is in points of the SH1E convention: 1 pt = 0.33 mm of em height,
// so 12 pt is a 4 mm face, which is the size upstream uses for seed words.
func textBlock(face *vector.Face, tb sh1e.TextBlock) (engrave.Plan, error) {
	mm := Params.Millimeter
	em := int(float64(tb.Size) * 0.33 * float64(mm))
	txt := strings.ToUpper(tb.Text) // the stroke face has no lowercase
	for _, r := range txt {
		if r == '\n' {
			continue
		}
		if _, _, ok := face.Decode(r); !ok {
			return nil, fmt.Errorf("the engraver's font has no glyph for %q", string(r))
		}
	}
	s := engrave.String(face, em, txt)
	sz := s.Measure()
	x := int(tb.XMM) * mm
	switch tb.Alignment {
	case sh1e.AlignCenter:
		x -= sz.X / 2
	case sh1e.AlignRight:
		x -= sz.X
	}
	p := engrave.Offset(x, int(tb.YMM)*mm, s.Engrave)
	return rotated(p, tb.Rotation, image.Pt(int(tb.XMM)*mm, int(tb.YMM)*mm)), nil
}

func rotated(p engrave.Plan, deg uint16, about image.Point) engrave.Plan {
	if deg == 0 {
		return p
	}
	rad := float64(deg) * math.Pi / 180
	return engrave.Offset(about.X, about.Y, engrave.Rotate(rad, engrave.Offset(-about.X, -about.Y, p)))
}

// ---- QR ---------------------------------------------------------------------------------

func qrBlock(q sh1e.QRBlock) (engrave.Plan, error) {
	levels := []qr.Level{qr.L, qr.M, qr.Q, qr.H}
	module := int(q.ModuleTenths) * Params.Millimeter / 10
	// engrave.QR draws each module as `scale` hatch lines one stroke-width apart, so the
	// module size on the plate is scale * StrokeWidth; pick the scale that gets closest.
	scale := int(math.Round(float64(module) / float64(Params.StrokeWidth)))
	if scale < 1 {
		scale = 1
	}
	p, err := engrave.QR(Params.StrokeWidth, scale, levels[q.Level], q.Data)
	if err != nil {
		return nil, fmt.Errorf("%d bytes do not fit a QR at level %d: %w", len(q.Data), q.Level, err)
	}
	mm := Params.Millimeter
	return engrave.Offset(int(q.XMM)*mm, int(q.YMM)*mm, p), nil
}

// QRSizeMM reports the edge length a block will have on the plate, for layout.
func QRSizeMM(q sh1e.QRBlock) (float64, error) {
	levels := []qr.Level{qr.L, qr.M, qr.Q, qr.H}
	code, err := qr.Encode(string(q.Data), levels[q.Level])
	if err != nil {
		return 0, err
	}
	module := int(q.ModuleTenths) * Params.Millimeter / 10
	scale := int(math.Round(float64(module) / float64(Params.StrokeWidth)))
	if scale < 1 {
		scale = 1
	}
	return float64(code.Size*scale*Params.StrokeWidth) / float64(Params.Millimeter), nil
}

// ---- SVG --------------------------------------------------------------------------------

// svgPath flattens one SVG `d` string (M/L/H/V/Q/C/Z, absolute or relative) into strokes.
// Units in the path are millimetres before ScalePct; curves are subdivided into 16 chords.
func svgPath(sp sh1e.SvgPath) (engrave.Plan, error) {
	segs, err := parsePath(sp.PathD)
	if err != nil {
		return nil, err
	}
	mm := float64(Params.Millimeter)
	k := float64(sp.ScalePct) / 100 * mm
	pt := func(x, y float64) image.Point { return image.Pt(int(math.Round(x*k)), int(math.Round(y*k))) }
	plan := func(yield func(engrave.Command)) {
		for _, s := range segs {
			switch s.op {
			case 'M':
				yield(engrave.Move(pt(s.p[0], s.p[1])))
			case 'L':
				yield(engrave.Line(pt(s.p[0], s.p[1])))
			}
		}
	}
	p := engrave.Offset(int(sp.XMM)*Params.Millimeter, int(sp.YMM)*Params.Millimeter, plan)
	return rotated(p, sp.Rotation, image.Pt(int(sp.XMM)*Params.Millimeter, int(sp.YMM)*Params.Millimeter)), nil
}

type seg struct {
	op byte // 'M' or 'L' after flattening
	p  [2]float64
}

const curveChords = 16

// parsePath is a small, strict SVG path parser for the SH1E subset.
func parsePath(d string) ([]seg, error) {
	var out []seg
	var cur, start, lastCtrl [2]float64
	var lastOp byte
	toks := tokenize(d)
	i := 0
	num := func() (float64, error) {
		if i >= len(toks) || toks[i].cmd != 0 {
			return 0, errors.New("svg path: expected a number")
		}
		v := toks[i].num
		i++
		return v, nil
	}
	nums := func(n int) ([]float64, error) {
		v := make([]float64, n)
		for j := range v {
			x, err := num()
			if err != nil {
				return nil, err
			}
			v[j] = x
		}
		return v, nil
	}
	emitLine := func(x, y float64) {
		out = append(out, seg{'L', [2]float64{x, y}})
		cur = [2]float64{x, y}
	}
	for i < len(toks) {
		t := toks[i]
		var cmd byte
		if t.cmd != 0 {
			cmd = t.cmd
			i++
		} else {
			// implicit repeat of the previous command (M becomes L per the SVG spec)
			cmd = lastOp
			if cmd == 'M' {
				cmd = 'L'
			} else if cmd == 'm' {
				cmd = 'l'
			}
			if cmd == 0 {
				return nil, errors.New("svg path: number before any command")
			}
		}
		rel := cmd >= 'a' && cmd <= 'z'
		up := cmd &^ 0x20
		base := [2]float64{}
		if rel {
			base = cur
		}
		switch up {
		case 'M':
			v, err := nums(2)
			if err != nil {
				return nil, err
			}
			cur = [2]float64{base[0] + v[0], base[1] + v[1]}
			start = cur
			out = append(out, seg{'M', cur})
		case 'L':
			v, err := nums(2)
			if err != nil {
				return nil, err
			}
			emitLine(base[0]+v[0], base[1]+v[1])
		case 'H':
			v, err := nums(1)
			if err != nil {
				return nil, err
			}
			emitLine(base[0]+v[0], cur[1])
		case 'V':
			v, err := nums(1)
			if err != nil {
				return nil, err
			}
			emitLine(cur[0], base[1]+v[0])
		case 'Q':
			v, err := nums(4)
			if err != nil {
				return nil, err
			}
			c := [2]float64{base[0] + v[0], base[1] + v[1]}
			e := [2]float64{base[0] + v[2], base[1] + v[3]}
			p0 := cur
			for s := 1; s <= curveChords; s++ {
				t := float64(s) / curveChords
				x := (1-t)*(1-t)*p0[0] + 2*(1-t)*t*c[0] + t*t*e[0]
				y := (1-t)*(1-t)*p0[1] + 2*(1-t)*t*c[1] + t*t*e[1]
				emitLine(x, y)
			}
			lastCtrl = c
		case 'C':
			v, err := nums(6)
			if err != nil {
				return nil, err
			}
			c1 := [2]float64{base[0] + v[0], base[1] + v[1]}
			c2 := [2]float64{base[0] + v[2], base[1] + v[3]}
			e := [2]float64{base[0] + v[4], base[1] + v[5]}
			p0 := cur
			for s := 1; s <= curveChords; s++ {
				t := float64(s) / curveChords
				u := 1 - t
				x := u*u*u*p0[0] + 3*u*u*t*c1[0] + 3*u*t*t*c2[0] + t*t*t*e[0]
				y := u*u*u*p0[1] + 3*u*u*t*c1[1] + 3*u*t*t*c2[1] + t*t*t*e[1]
				emitLine(x, y)
			}
			lastCtrl = c2
		case 'Z':
			if cur != start {
				emitLine(start[0], start[1])
			}
		default:
			return nil, fmt.Errorf("svg path: command %q is outside the M/L/H/V/Q/C/Z subset", string(cmd))
		}
		_ = lastCtrl
		lastOp = cmd
	}
	return out, nil
}

type token struct {
	cmd byte
	num float64
}

func tokenize(d string) []token {
	var out []token
	i := 0
	for i < len(d) {
		c := d[i]
		switch {
		case c == ' ' || c == ',' || c == '\n' || c == '\t' || c == '\r':
			i++
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
			out = append(out, token{cmd: c})
			i++
		default:
			j := i
			if d[j] == '-' || d[j] == '+' {
				j++
			}
			seenDot, seenExp := false, false
			for j < len(d) {
				ch := d[j]
				if ch >= '0' && ch <= '9' {
					j++
				} else if ch == '.' && !seenDot && !seenExp {
					seenDot = true
					j++
				} else if (ch == 'e' || ch == 'E') && !seenExp {
					seenExp = true
					j++
					if j < len(d) && (d[j] == '-' || d[j] == '+') {
						j++
					}
				} else {
					break
				}
			}
			if j == i {
				i++ // unparseable byte: skip it rather than loop forever
				continue
			}
			var v float64
			fmt.Sscanf(d[i:j], "%g", &v)
			out = append(out, token{num: v})
			i = j
		}
	}
	return out
}

// Preview renders a Result as a plate-anchored SVG: the plate chrome, then every pen-down
// stroke as a line. Used by the composer and by Any Two Keys; what it shows is the plan.
func Preview(r Result) string {
	mm := float64(Params.Millimeter)
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" preserveAspectRatio="xMidYMid meet">`, r.DimsMM.X, r.DimsMM.Y)
	fmt.Fprintf(&sb, `<rect x="0.5" y="0.5" width="%d" height="%d" rx="3" ry="3" fill="#ececec" stroke="#444" stroke-width="0.4"/>`, r.DimsMM.X-1, r.DimsMM.Y-1)
	fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="#999" stroke-width="0.15" stroke-dasharray="0.6,0.6"/>`,
		OuterMarginMM, OuterMarginMM, r.DimsMM.X-2*OuterMarginMM, r.DimsMM.Y-2*OuterMarginMM)
	for _, h := range holes(r.Plate) {
		fmt.Fprintf(&sb, `<circle cx="%g" cy="%g" r="2.2" fill="none" stroke="#999" stroke-width="0.2"/>`, h[0], h[1])
	}
	sb.WriteString(`<path fill="none" stroke="#111" stroke-width="0.3" stroke-linecap="round" d="`)
	var pen image.Point
	r.Plan(func(c engrave.Command) {
		if c.Line {
			fmt.Fprintf(&sb, "M%.2f %.2fL%.2f %.2f", float64(pen.X)/mm, float64(pen.Y)/mm, float64(c.Coord.X)/mm, float64(c.Coord.Y)/mm)
		}
		pen = c.Coord
	})
	sb.WriteString(`"/></svg>`)
	return sb.String()
}

// holes are where the mounting bolts go through, for the preview.
func holes(t sh1e.PlateType) [][2]float64 {
	dims, _ := Dims(t)
	w, h := float64(dims.X), float64(dims.Y)
	out := [][2]float64{{5, 5}, {w - 5, 5}, {5, h - 5}, {w - 5, h - 5}}
	if t == sh1e.LargePlate {
		out = append(out, [2]float64{5, h / 2}, [2]float64{w - 5, h / 2})
	}
	return out
}
