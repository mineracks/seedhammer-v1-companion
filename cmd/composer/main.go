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
	"github.com/mineracks/seedhammer-v1-companion/font/constant"
	"github.com/mineracks/seedhammer-v1-companion/font/vector"
)

const composerVersion = "v0.1-phase1-milestone"

func main() {
	js.Global().Set("composerVersion", js.FuncOf(exportVersion))
	js.Global().Set("composerPlateTypes", js.FuncOf(exportPlateTypes))
	js.Global().Set("composerEncodeText", js.FuncOf(exportEncodeText))
	js.Global().Set("composerPreviewText", js.FuncOf(exportPreviewText))
	js.Global().Set("composerQR", js.FuncOf(exportQR))
	js.Global().Set("composerPreviewSVG", js.FuncOf(exportPreviewSVG))
	js.Global().Set("composerEncodeSVG", js.FuncOf(exportEncodeSVG))
	js.Global().Set("composerQRSVG", js.FuncOf(exportQRSVG))
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
	// SVG y-axis grows downward — consistent with our XMM/YMM "from
	// plate-origin top-left" convention.
	fmt.Fprintf(&sb,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g %g" preserveAspectRatio="xMidYMid meet">`,
		dims.W, dims.H,
	)
	writePlateChrome(&sb, dims)

	// Text blocks — render via the same vector engraving face the v1
	// firmware streams to the MarkingWay head. Every <path> stroke in
	// the preview is a stroke the engraver would actually punch.
	for _, l := range layoutLines(lines) {
		renderTextRow(&sb, l)
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}

// ─── SVG-mode exports ────────────────────────────────────────────────────
//
// In SVG mode the composer takes a list of SVG path d-strings (extracted
// by the JS shell from an uploaded .svg file) and stamps them onto the
// plate. Each d-string becomes one sh1e.SvgPath, scaled to fit the plate
// body and anchored at the plate origin.

// readSVGArgs extracts (plateType, []d-strings) from JS args.
func readSVGArgs(args []js.Value) (sh1e.PlateType, []string, error) {
	if len(args) != 2 {
		return 0, nil, fmt.Errorf("expected (plateType, paths[]), got %d args", len(args))
	}
	plateType := sh1e.PlateType(args[0].Int())
	jsPaths := args[1]
	n := jsPaths.Length()
	paths := make([]string, 0, n)
	for i := 0; i < n; i++ {
		s := jsPaths.Index(i).String()
		if s == "" {
			continue
		}
		paths = append(paths, s)
	}
	return plateType, paths, nil
}

// makeSVGDesign builds an sh1e.Design with one SvgPath per d-string. For
// Phase 1 we anchor every path at the plate body's top-left interior
// corner (innerMargin, innerMargin) at 100% scale; richer positioning
// arrives in a follow-up commit once the composer has on-plate drag UI.
func makeSVGDesign(plateType sh1e.PlateType, paths []string) sh1e.Design {
	svgPaths := make([]sh1e.SvgPath, 0, len(paths))
	for _, d := range paths {
		svgPaths = append(svgPaths, sh1e.SvgPath{
			XMM:      innerMarginMM,
			YMM:      innerMarginMM,
			ScalePct: 100,
			PathD:    d,
		})
	}
	return sh1e.Design{
		PlateType: plateType,
		SvgPaths:  svgPaths,
	}
}

// exportPreviewSVG: composerPreviewSVG(plateType, pathDStrings) -> string
//
// Renders the plate outline + the supplied SVG paths, rendered native via
// the browser's own <path d="..."/> support. Each path is drawn at the
// anchor (innerMargin, innerMargin) at 100% scale.
func exportPreviewSVG(this js.Value, args []js.Value) any {
	plateType, paths, err := readSVGArgs(args)
	if err != nil {
		return jsError(err)
	}
	if int(plateType) >= len(plateDimsByID) {
		return jsError(fmt.Errorf("unknown plate type %d", plateType))
	}
	dims := plateDimsByID[plateType]

	var sb strings.Builder
	fmt.Fprintf(&sb,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g %g" preserveAspectRatio="xMidYMid meet">`,
		dims.W, dims.H,
	)
	writePlateChrome(&sb, dims)
	for _, d := range paths {
		// Each path is placed at (innerMargin, innerMargin) via a translate.
		fmt.Fprintf(&sb,
			`<g transform="translate(%g %g)" fill="none" stroke="#111" stroke-width="0.3" stroke-linecap="round" stroke-linejoin="round"><path d="%s"/></g>`,
			float64(innerMarginMM), float64(innerMarginMM), escapeXML(d),
		)
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// exportEncodeSVG: composerEncodeSVG(plateType, pathDStrings) -> Uint8Array
func exportEncodeSVG(this js.Value, args []js.Value) any {
	plateType, paths, err := readSVGArgs(args)
	if err != nil {
		return jsError(err)
	}
	bytes, err := sh1e.Encode(makeSVGDesign(plateType, paths))
	if err != nil {
		return jsError(err)
	}
	return uint8Array(bytes)
}

// exportQRSVG: composerQRSVG(plateType, pathDStrings) -> {svg, modules, bytes}
func exportQRSVG(this js.Value, args []js.Value) any {
	plateType, paths, err := readSVGArgs(args)
	if err != nil {
		return jsError(err)
	}
	payload, err := sh1e.Encode(makeSVGDesign(plateType, paths))
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

// holeInsetMM is the distance from each plate edge to the centre of each
// M3 mounting hole. Verified against the Mineracks SH-02 production DXF
// (Name-Plate-85x85-316L 2B.DXF): 4 circles at (3,3), (3,82), (82,3),
// (82,82), radius 1.5mm. The centres sit exactly on the outerMargin
// line. Same value used for SH-01 and SH-03 — same physical drill jig.
const holeInsetMM = 3.0

// holeDiameterMM matches the M3 clearance hole in the CAD: 3.0mm
// diameter (radius 1.5mm). The hole renders identically to the
// production part — no visual fudging.
const holeDiameterMM = 3.0

// holeDangerDiameterMM is the diameter of the dashed "no-engrave"
// exclusion ring drawn around each hole. Picked so the ring's outer
// edge sits at ~6mm from plate edge — gives the user a clear "don't
// engrave inside the M3 nut footprint" margin (M3 nut across-flats is
// 5.5mm, head/socket 5-6mm).
const holeDangerDiameterMM = 7.0

// sh03MidHoleY is the Y coordinate of the two extra clamping holes on
// the long sides of the SH-03 (Large) plate, measured from the top edge.
// User-confirmed at ~45 mm — upper-third position, not the geometric
// centerline. Matches the physical clamping pattern on real Mineracks
// SH-03 production plates (see engraved sample in roadmap photos).
const sh03MidHoleY = 45.0

// holePositions returns the mounting-hole centres for a given plate.
// SH-01 (Small) and SH-02 (Square) have 4 corner holes; SH-03 (Large)
// adds 2 mid-edge holes on the long sides at sh03MidHoleY for extra
// clamping force (the 134mm length needs more than 4-corner support).
func holePositions(dims plateDims) [][2]float64 {
	w, h := dims.W, dims.H
	corners := [][2]float64{
		{holeInsetMM, holeInsetMM},
		{w - holeInsetMM, holeInsetMM},
		{w - holeInsetMM, h - holeInsetMM},
		{holeInsetMM, h - holeInsetMM},
	}
	// Threshold: any plate taller than ~120mm is Large-class and gets
	// the two extra mid-edge holes. SH-03 is 134mm; SH-02 is 85mm so it
	// falls below the threshold.
	if h >= 120 {
		corners = append(corners,
			[2]float64{holeInsetMM, sh03MidHoleY},
			[2]float64{w - holeInsetMM, sh03MidHoleY},
		)
	}
	return corners
}

// writePlateChrome emits the plate outline + margin guides + mounting
// holes into sb. Shared between Text-mode and SVG-mode previews.
func writePlateChrome(sb *strings.Builder, dims plateDims) {
	// Plate body.
	fmt.Fprintf(sb,
		`<rect x="0.5" y="0.5" width="%g" height="%g" rx="3" ry="3" fill="#ececec" stroke="#444" stroke-width="0.4"/>`,
		dims.W-1, dims.H-1,
	)
	// outer-margin guide (no-engrave boundary at 3mm)
	fmt.Fprintf(sb,
		`<rect x="%g" y="%g" width="%g" height="%g" fill="none" stroke="#999" stroke-width="0.15" stroke-dasharray="0.6,0.6"/>`,
		outerMarginMM, outerMarginMM, dims.W-2*outerMarginMM, dims.H-2*outerMarginMM,
	)
	// inner-margin guide (safe text area at 10mm)
	fmt.Fprintf(sb,
		`<rect x="%g" y="%g" width="%g" height="%g" fill="none" stroke="#666" stroke-width="0.15" stroke-dasharray="0.4,0.4"/>`,
		innerMarginMM, innerMarginMM, dims.W-2*innerMarginMM, dims.H-2*innerMarginMM,
	)
	// Mounting holes — drawn LAST so they sit on top of the margin guides.
	// Each hole gets a dashed "danger" exclusion ring + the hole itself,
	// rendered with a contrasting fill so it reads as "metal removed
	// here, leave clear" at a glance.
	for _, h := range holePositions(dims) {
		// no-engrave exclusion ring (dashed red)
		fmt.Fprintf(sb,
			`<circle cx="%g" cy="%g" r="%g" fill="none" stroke="#c92a2a" stroke-width="0.15" stroke-dasharray="0.5,0.5" opacity="0.7"/>`,
			h[0], h[1], holeDangerDiameterMM/2,
		)
		// the actual hole — fill matches the page bg so the eye reads
		// "metal removed here" rather than a printed dot
		fmt.Fprintf(sb,
			`<circle cx="%g" cy="%g" r="%g" fill="#fff" stroke="#444" stroke-width="0.25"/>`,
			h[0], h[1], holeDiameterMM/2,
		)
	}
}

// faceForFont returns the vector engraving face used for a given SH1E
// font ID. Every SH1E font currently maps to font/constant — that's the
// only outline font the v1 firmware ships, and it's the one the engraver
// actually punches strokes from. Bitmap faces (font/comfortaa,
// font/poppins) are LCD-only and not used here.
//
// When upstream lands more vector faces, this map grows. The composer's
// preview is therefore guaranteed to look like what the engraver
// produces, because it's literally walking the same segment data the
// engrave pipeline does.
func faceForFont(id sh1e.FontID) *vector.Face {
	// Single face for now. Future: switch on id.
	_ = id
	return constant.Font
}

// renderTextRow emits an SVG <g> containing the stroked outline of a
// single text line. Uses the vector engraving face — every segment is a
// MoveTo or LineTo, identical to what the engraver's stepper will follow.
//
// (l.XMM, l.YMM) is treated as the TOP-LEFT of the glyph cell. The
// translate target is the baseline = YMM + ascent*scale, so glyphs
// render with their top edge at YMM.
//
// vector-effect="non-scaling-stroke" keeps the visible stroke width
// constant regardless of the scale transform, matching the engraver's
// fixed punch dot size.
func renderTextRow(sb *strings.Builder, l lineLayout) {
	face := faceForFont(l.FontID)
	if face == nil {
		return
	}
	metrics := face.Metrics()
	emHeight := float64(metrics.Height)
	if emHeight <= 0 {
		return
	}
	fontMM := float64(l.Size) * 0.33
	scale := fontMM / emHeight

	// First pass: total advance for horizontal alignment math.
	totalAdvance := 0
	for _, r := range l.Text {
		adv, _, ok := face.Decode(r)
		if ok {
			totalAdvance += adv
		}
	}

	originX := float64(l.XMM)
	switch l.Alignment {
	case sh1e.AlignCenter:
		originX -= float64(totalAdvance) * scale * 0.5
	case sh1e.AlignRight:
		originX -= float64(totalAdvance) * scale
	}
	baselineY := float64(l.YMM) + float64(metrics.Ascent)*scale

	fmt.Fprintf(sb,
		`<g transform="translate(%g %g) scale(%g)" fill="none" stroke="#111" stroke-width="0.8" stroke-linecap="round" stroke-linejoin="round" vector-effect="non-scaling-stroke">`,
		originX, baselineY, scale,
	)

	cursorX := 0
	for _, r := range l.Text {
		adv, segs, ok := face.Decode(r)
		if !ok {
			continue
		}
		var d strings.Builder
		for {
			seg, hasMore := segs.Next()
			if !hasMore {
				break
			}
			switch seg.Op {
			case vector.SegmentOpMoveTo:
				fmt.Fprintf(&d, "M%d %d ", cursorX+seg.Arg.X, seg.Arg.Y)
			case vector.SegmentOpLineTo:
				fmt.Fprintf(&d, "L%d %d ", cursorX+seg.Arg.X, seg.Arg.Y)
			}
		}
		if d.Len() > 0 {
			fmt.Fprintf(sb, `<path d="%s"/>`, d.String())
		}
		cursorX += adv
	}
	sb.WriteString(`</g>`)
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
