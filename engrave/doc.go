// Package engrave transforms shapes such as text and QR codes into line
// and move commands for the engraver.
//
// LIFTED from upstream seedhammer/seedhammer at v1.3.0
// (commit 2f071c1d8f23eb7fd39b15fc0acb8874113f801e). Hardware-agnostic in
// the sense that it produces command streams that any compatible engraver
// can consume; the actual byte-level wire encoding lives in
// engrave/wire/ + driver/mjolnir/.
//
// Subpackages:
//   - engrave/wire/sh1e/  — the SH1E plate-design envelope (new code, ours)
//   - engrave/wire/       — the live MarkingWay USB-serial encoder (future)
package engrave
