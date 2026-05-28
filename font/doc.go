// Package font provides the v1 controller's pre-rasterised engraver fonts.
//
// The engraver itself knows nothing about glyphs — the Pi controller
// rasterises every character to MoveTo/LineTo segments before sending bytes
// down the USB-serial pipe. These fonts (Comfortaa, Poppins, and a
// constant-width fallback) are pre-baked OpenType segments compiled into
// the Go binary.
//
// Subdirectories carry the actual glyph data:
//
//   - font/comfortaa/  — Comfortaa font, pre-rasterised
//   - font/poppins/    — Poppins font, pre-rasterised
//   - font/constant/   — constant-width fallback font
//
// Character set is ASCII-only. Lifting non-ASCII glyphs requires either
// new fonts upstream or a deliberate decision to add Unicode support to
// the v1 engrave pipeline (out of scope for this companion port).
//
// Status: STUB — directory tree mirrors upstream layout; glyph data to be
// lifted verbatim from upstream seedhammer/seedhammer at v1.3.0.
package font
