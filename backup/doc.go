// Package backup defines SeedHammer v1 plate dimensions and the backup-
// encoding scheme that turns wallet data into engrave-ready descriptions.
//
// Three plate types match upstream's stainless plate SKUs:
//
//	SmallPlate   85 × 55 mm   — single seed (12 or 24 words)
//	SquarePlate  85 × 85 mm   — single seed + title
//	LargePlate   85 × 134 mm  — seed + descriptor for multisig
//
// LIFTED from upstream seedhammer/seedhammer at v1.3.0
// (commit 2f071c1d8f23eb7fd39b15fc0acb8874113f801e). NOT lifted from
// Gangleri42's fork because that fork strips out SmallPlate (SH-II
// doesn't support it) and removes the UR-code multi-plate encoding
// (SH-II uses NFC payload instead).
//
// backup_test.go from upstream pulls in mjolnir + engrave + bip32 — we've
// renamed it to backup_test.go.deferred until those packages land.
package backup
