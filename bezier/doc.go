// Package bezier provides quadratic and cubic Bezier curve tessellation
// for the engrave pipeline.
//
// Used to convert OpenType glyph QuadTo/CubeTo segments and SH1E SVG path
// Q/C commands into linear MoveTo/LineTo sequences at the engraver's
// resolution.
//
// Status: STUB — to be lifted from upstream at v1.3.0 + cross-checked
// against Gangleri42's fork (the math is hardware-agnostic and unchanged).
package bezier
