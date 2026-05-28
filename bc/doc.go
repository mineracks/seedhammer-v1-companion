// Package bc is the parent of upstream's Blockchain Commons code: UR
// (Uniform Resources) encoded payloads, fountain codes for multi-frame
// transport, bytewords for human-checksummable encoding, plus xoshiro256
// for deterministic ranom-stream generation.
//
// All five subpackages (ur, fountain, bytewords, urtypes, xoshiro256) live
// here. The v1 controller uses these to encode multi-plate backup data
// into animated QR codes that fit through the WaveShare LCD's small frame.
//
// LIFTED from upstream seedhammer/seedhammer at v1.3.0
// (commit 2f071c1d8f23eb7fd39b15fc0acb8874113f801e). Gangleri42's fork
// has the same code modernized for Go 1.23+ idioms; functionally identical.
// We use upstream's v1.3.0 because that's what backup/ was tested against.
package bc
