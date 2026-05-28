// Package bezier provides quadratic and cubic Bezier curve representations
// plus tessellation helpers for the engrave pipeline.
//
// Used to convert OpenType glyph QuadTo/CubeTo segments and SH1E SVG path
// Q/C commands into linear MoveTo/LineTo sequences at the engraver's
// resolution.
//
// LIFTED from https://github.com/Gangleri42/seedhammer/tree/seedhammer-features/bezier
// at commit 0a3c63efb125d17d8ec86ce739ecd058c8747cfe. Hardware-agnostic
// math; identical to upstream apart from the Go import path.
package bezier
