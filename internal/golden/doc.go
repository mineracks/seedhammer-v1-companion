// Package golden is a test-fixture provider used by both the composer-side
// preview WASM and the emulator to assert that rendered output is identical
// to reference goldens.
//
// Golden images, golden SH1E byte sequences, and golden engrave command
// streams all live here. CI compares live rendering against these.
//
// Status: STUB — fixtures generated progressively as features land. The
// SH1E reference encoder will be tested against the documented examples
// in docs/architecture/sh1e-spec.md.
package golden
