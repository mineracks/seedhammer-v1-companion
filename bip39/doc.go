// Package bip39 is the BIP39 mnemonic seed-phrase encoder/decoder + the
// 2048-word English wordlist baked in as data.
//
// Used by:
//   - The composer to take a user-typed/scanned seed and validate it
//     before designing a plate
//   - The Pi controller during the actual engrave flow
//   - The SeedSigner sim bridge for the QR-handoff bus
//
// LIFTED from upstream seedhammer/seedhammer at v1.3.0
// (commit 2f071c1d8f23eb7fd39b15fc0acb8874113f801e). Hardware-agnostic;
// the v1.3.0 baseline matches what backup/ was tested against.
//
// gen.go has //go:build ignore — it's the wordlist generator, run via
// `go generate`, not part of the build.
package bip39
