//go:build js && wasm

// Command composer is the browser-side plate composer for SeedHammer v1.
//
// Compiles to WebAssembly via:
//
//	GOOS=js GOARCH=wasm go build -o ./web/composer/composer.wasm ./cmd/composer
//
// The static shell (HTML/CSS/JS) lives under web/composer/. Exported JS
// surface is documented in web/composer/app.js — every JS function listed
// there is bound here.
package main

import (
	"fmt"
	"strings"
	"syscall/js"

	"github.com/kortschak/qr"
	"github.com/mineracks/seedhammer-v1-companion/engrave/wire/sh1e"
)

const composerVersion = "v0.1-phase1-milestone"

func main() {
	js.Global().Set("composerVersion", js.FuncOf(exportVersion))
	js.Global().Set("composerPlateTypes", js.FuncOf(exportPlateTypes))
	js.Global().Set("composerEncodeText", js.FuncOf(exportEncodeText))
	js.Global().Set("composerPreviewText", js.FuncOf(exportPreviewText))
	js.Global().Set("composerQR", js.FuncOf(exportQR))
	// Block forever so the Go runtime keeps the exported funcs alive.
	select {}
}

// ─── Plate geometry (v1 hardware constants) ──────────────────────────────

type plateDims struct {
	Name string
	W    float64 // mm
	H    float64 // mm
}

// plateDimsByID lists v1 plate sizes from upstream backup/backup.go at
// v1.3.0. SmallPlate / SquarePlate / LargePlate enum order matches sh1e's.
var plateDimsByID = []plateDims{
	{Name: "Small", W: 85, H: 55},
	{Name: "Square", W: 85, H: 85},
	{Name: "Large", W: 85, H: 134},
}

// outerMarginMM matches backup.go's outerMargin — the "no-engrave"
// boundary inset from the plate edge.
const outerMarginMM = 3.0

// innerMarginMM matches backup.go's innerMargin — the slightly larger
// inset where text typically starts. Used as a guide rectangle in the
// preview.
const innerMarginMM = 10.0

// ─── Layout constants ─────────────────────────────────────────────────────

const (
	// Default text-block font size in points.
	defaultFontSizePoints = 12
	// X position of the first text block — sits inside the innerMargin
	// guide so the engraver never touches the no-engrave zone.
	textXMM = 11
	// Y of the first text block — also inside innerMargin.
	textYStartMM = 11
	// Vertical stride between block tops.
	textYStrideMM = 8
)

// maxLinesFor returns the number of text blocks that fit inside the
// innerMargin-safe area for a given plate. Used by the JS shell to size
// the line-input UI per plate.
func maxLinesFor(p plateDims) int {
	// Last line's bottom must be ≤ p.H - innerMarginMM. With each line
	// occupying ~fontMM of vertical space starting at textYStartMM + i*stride:
	//
	//   textYStartMM + i*textYStrideMM + fontHeight ≤ p.H - innerMarginMM
	//
	// Approximate fontHeight as textYStrideMM-1 (leaves 1mm of breathing
	// room between lines).
	available := p.H - innerMarginMM - textYStartMM
	if available <= 0 {
		return 0
	}
	n := int(available/textYStrideMM) + 1
	if n < 1 {
		return 1
	}
	// Hard upper bound from sh1e spec.
	if n > 32 {
		n = 32
	}
	return n
}

// ─── Shared layout ───────────────────────────────────────────────────────
//
// Encode and Preview must agree on where text lands. layoutLines is the
// single source of truth so a change here propagates to both.

type lineLayout struct {
	FontID    sh1e.FontID
	Size      uint16 // points
	XMM       int16
	YMM       int16
	Alignment sh1e.Alignment
	Text      string
}

func layoutLines(lines []string) []lineLayout {
	out := make([]lineLayout, 0, len(lines))
	for i, line := range lines {
		out = append(out, lineLayout{
			FontID:    sh1e.FontComfortaa,
			Size:      defaultFontSizePoints,
			XMM:       textXMM,
			YMM:       int16(textYStartMM + i*textYStrideMM),
			Alignment: sh1e.AlignLeft,
			Text:      line,
		})
	}
	return out
}

// ─── JS exports ──────────────────────────────────────────────────────────

func exportVersion(this js.Value, args []js.Value) any {
	return composerVersion
}

func exportPlateTypes(this js.Value, args []js.Value) any {
	out := make([]any, 0, len(plateDimsByID))
	for i, p := range plateDimsByID {
		out = append(out, map[string]any{
			"id":        i,
			"name":      p.Name,
			"w_mm":      p.W,
			"h_mm":      p.H,
			"max_lines": maxLinesFor(p),
		})
	}
	return js.ValueOf(out)
}

// exportEncodeText: composerEncodeText(plateType:number, lines:string[]) -> Uint8Array
func exportEncodeText(this js.Value, args []js.Value) any {
	plateType, lines, err := readArgs(args)
	if err != nil {
		return jsError(err)
	}
	layout := layoutLines(lines)
	blocks := make([]sh1e.TextBlock, 0, len(layout))
	for _, l := range layout {
		blocks = append(blocks, sh1e.TextBlock{
			FontID:    l.FontID,
			Size:      l.Size,
			XMM:       l.XMM,
			YMM:       l.YMM,
			Alignment: l.Alignment,
			Text:      l.Text,
		})
	}
	bytes, err := sh1e.Encode(sh1e.Design{
		PlateType:  plateType,
		TextBlocks: blocks,
	})
	if err != nil {
		return jsError(err)
	}
	return uint8Array(bytes)
}

// exportPreviewText: composerPreviewText(plateType:number, lines:string[]) -> string (SVG)
//
// Returns a plate-anchored inline SVG showing the layout. Phase 1 milestone
// uses CSS-side font rendering (monospaced web font) rather than glyph-
// faithful Comfortaa rasterisation — pixel-perfect preview comes in a
// follow-up commit. Coordinates exactly match what exportEncodeText emits.
func exportPreviewText(this js.Value, args []js.Value) any {
	plateType, lines, err := readArgs(args)
	if err != nil {
		return jsError(err)
	}
	if int(plateType) >= len(plateDimsByID) {
		return jsError(fmt.Errorf("unknown plate type %d", plateType))
	}
	dims := plateDimsByID[plateType]

	var sb strings.Builder
	// Note: SVG y-axis grows downward (consistent with our XMM/YMM
	// "from plate-origin top-left" convention).
	fmt.Fprintf(&sb,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g %g" preserveAspectRatio="xMidYMid meet" font-family="ui-monospace, SFMono-Regular, Menlo, monospace">`,
		dims.W, dims.H,
	)

	// Plate outline (rounded-corner rect)
	fmt.Fprintf(&sb,
		`<rect x="0.5" y="0.5" width="%g" height="%g" rx="3" ry="3" fill="#ececec" stroke="#444" stroke-width="0.4"/>`,
		dims.W-1, dims.H-1,
	)

	// outerMargin guide (dashed, light)
	fmt.Fprintf(&sb,
		`<rect x="%g" y="%g" width="%g" height="%g" fill="none" stroke="#999" stroke-width="0.15" stroke-dasharray="0.6,0.6"/>`,
		outerMarginMM, outerMarginMM, dims.W-2*outerMarginMM, dims.H-2*outerMarginMM,
	)
	// innerMargin guide (dashed, slightly darker)
	fmt.Fprintf(&sb,
		`<rect x="%g" y="%g" width="%g" height="%g" fill="none" stroke="#666" stroke-width="0.15" stroke-dasharray="0.4,0.4"/>`,
		innerMarginMM, innerMarginMM, dims.W-2*innerMarginMM, dims.H-2*innerMarginMM,
	)

	// Text blocks
	for _, l := range layoutLines(lines) {
		// font-size in SVG units = mm (because of viewBox). Roughly:
		// 12 pt ≈ 4.23 mm; we use 0.33 mm/pt as the multiplier.
		fontMM := float64(l.Size) * 0.33
		anchor := "start"
		switch l.Alignment {
		case sh1e.AlignCenter:
			anchor = "middle"
		case sh1e.AlignRight:
			anchor = "end"
		}
		// We treat (XMM, YMM) as the TOP-LEFT corner of the glyph cell
		// (matching the spec). SVG <text> y is the baseline. Offset by
		// the cap height (~0.78 of font size for monospaced faces) so
		// the rendered text starts at YMM. Avoids dominant-baseline,
		// which Safari renders inconsistently for "hanging".
		baselineY := float64(l.YMM) + fontMM*0.78
		fmt.Fprintf(&sb,
			`<text x="%d" y="%g" font-size="%g" text-anchor="%s" fill="#111" font-weight="600">%s</text>`,
			l.XMM, baselineY, fontMM, anchor, escapeXML(l.Text),
		)
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}

// exportQR: composerQR(plateType:number, lines:string[]) -> {svg:string, modules:number, bytes:number}
//
// Encodes the design as SH1E, then encodes those bytes as a QR code at
// medium error-correction level (15% damage tolerance — a sensible balance
// for a phone-screen-to-camera handoff). The returned SVG is a full QR
// matrix, drawn as one rect per dark module against a white background.
//
// JS uses this to show a scannable QR to the SeedHammer v1 camera.
func exportQR(this js.Value, args []js.Value) any {
	plateType, lines, err := readArgs(args)
	if err != nil {
		return jsError(err)
	}
	layout := layoutLines(lines)
	blocks := make([]sh1e.TextBlock, 0, len(layout))
	for _, l := range layout {
		blocks = append(blocks, sh1e.TextBlock{
			FontID:    l.FontID,
			Size:      l.Size,
			XMM:       l.XMM,
			YMM:       l.YMM,
			Alignment: l.Alignment,
			Text:      l.Text,
		})
	}
	payload, err := sh1e.Encode(sh1e.Design{
		PlateType:  plateType,
		TextBlocks: blocks,
	})
	if err != nil {
		return jsError(err)
	}

	code, err := qr.Encode(string(payload), qr.M)
	if err != nil {
		return jsError(fmt.Errorf("qr encode: %w", err))
	}
	return js.ValueOf(map[string]any{
		"svg":     qrSVG(code),
		"modules": code.Size,
		"bytes":   len(payload),
	})
}

// qrSVG renders a kortschak/qr Code as an inline SVG string. Uses
// viewBox = module-count so callers can size by CSS without distortion.
// One <rect> per dark module, against a white background.
func qrSVG(code *qr.Code) string {
	dim := code.Size
	// Quiet zone: per spec, 4 modules of white margin around the QR. We
	// embed it into the viewBox so the consumer doesn't have to add padding.
	const quiet = 4
	total := dim + 2*quiet

	var sb strings.Builder
	fmt.Fprintf(&sb,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`,
		total, total,
	)
	// White background covers the whole canvas including the quiet zone.
	fmt.Fprintf(&sb, `<rect width="%d" height="%d" fill="#fff"/>`, total, total)
	// Black modules. Coalesce consecutive horizontal runs into a single rect
	// to shrink the SVG payload — typical QR has ~50% dark modules; per-cell
	// rects would be ~half the matrix count.
	for y := 0; y < dim; y++ {
		x := 0
		for x < dim {
			if !code.Black(x, y) {
				x++
				continue
			}
			runStart := x
			for x < dim && code.Black(x, y) {
				x++
			}
			runLen := x - runStart
			fmt.Fprintf(&sb,
				`<rect x="%d" y="%d" width="%d" height="1" fill="#000"/>`,
				runStart+quiet, y+quiet, runLen,
			)
		}
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// ─── Helpers ─────────────────────────────────────────────────────────────

func readArgs(args []js.Value) (sh1e.PlateType, []string, error) {
	if len(args) != 2 {
		return 0, nil, fmt.Errorf("expected 2 args, got %d", len(args))
	}
	plateType := sh1e.PlateType(args[0].Int())
	jsLines := args[1]
	n := jsLines.Length()
	lines := make([]string, 0, n)
	for i := 0; i < n; i++ {
		s := jsLines.Index(i).String()
		if s == "" {
			continue
		}
		lines = append(lines, s)
	}
	return plateType, lines, nil
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func jsError(err error) any {
	jsErr := js.Global().Get("Error").New(err.Error())
	panic(jsErr)
}

func uint8Array(src []byte) js.Value {
	dst := js.Global().Get("Uint8Array").New(len(src))
	js.CopyBytesToJS(dst, src)
	return dst
}
