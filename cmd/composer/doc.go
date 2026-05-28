// Command composer is the browser-side plate composer for SeedHammer v1.
//
// Compiles to WebAssembly via GOOS=js GOARCH=wasm. The HTML/CSS shell that
// hosts this WASM lives under web/composer/.
//
// Responsibilities:
//   - Render a real-time preview of the plate using the same engrave + font
//     code the Pi controller runs (code-identity guarantees preview = reality).
//   - Emit an SH1E envelope (see docs/architecture/sh1e-spec.md) that the
//     Pi controller can scan via QR code.
//
// Exports four JS functions, modelled on Gangleri42's cmd/webnfc:
//
//	seedhammerEncodeText(plateType uint8, blocks []TextBlock) []byte // SH1E
//	seedhammerPreviewText(plateType uint8, blocks []TextBlock) string // SVG preview
//	seedhammerEncodeSVG(plateType uint8, paths []SvgPath) []byte // SH1E
//	seedhammerPreviewSVG(plateType uint8, paths []SvgPath) string // SVG preview
//
// Status: STUB — implementation lifted progressively from
// https://github.com/Gangleri42/seedhammer/tree/seedhammer-features/cmd/webnfc
// at pinned baseline 0a3c63efb125d17d8ec86ce739ecd058c8747cfe, with the
// SH-II geometry block replaced by v1 constants from upstream
// seedhammer/seedhammer v1.3.0 (backup/backup.go).
package main
