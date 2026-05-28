// Package backup defines SeedHammer v1 plate dimensions and layout
// constants.
//
// Three plate types are supported, matching upstream's stainless plate SKUs:
//
//	SmallPlate   85 × 55 mm   — single seed (12 or 24 words)
//	SquarePlate  85 × 85 mm   — single seed + title
//	LargePlate   85 × 134 mm  — seed + descriptor for multisig
//
// The engraver's origin sits 97mm in the X axis from the plate edge (a
// fixed offset of the physical machine). outerMargin = 3 mm and
// innerMargin = 10 mm define the engrave-safe area.
//
// All constants are mm; the engrave/ package converts to machine steps
// (1 step ≈ 0.00796 mm) at command-generation time.
//
// Status: STUB — to be lifted verbatim from upstream's backup/backup.go
// at v1.3.0. The constants are stable across all v1.x releases.
package backup
