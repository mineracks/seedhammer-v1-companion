// Package engrave converts plate designs into MarkingWay engraver commands.
//
// Wraps three concerns:
//
//   - Geometry: take a layout (text + SVG paths + plate type) and produce
//     a stream of MoveTo/LineTo commands in the engraver's coordinate
//     system (machine steps, 1 step ≈ 0.00796 mm).
//
//   - Tessellation: convert higher-order curves (Quad/Cube/Spline) into
//     line segments at the engraver's resolution. Uses bezier/ + bspline/.
//
//   - Wire: serialise the command stream into the 10-byte binary frames
//     the MarkingWay protocol expects. See engrave/wire/ for the on-the-wire
//     formats (engraver USB-serial as well as SH1E for QR transport).
//
// Status: STUB — to be lifted from upstream seedhammer/seedhammer at v1.3.0
// (commit 2f071c1d8f23eb7fd39b15fc0acb8874113f801e), specifically the
// engrave package. See docs/architecture/v1-engrave-spec.md for the
// audit of the v1 wire protocol that this package targets.
package engrave
