// Package wire is the parent for v1 wire formats.
//
// Two sub-protocols share this directory:
//
//   - wire/    (this dir, future): the live MarkingWay USB-serial protocol
//     spoken by the Pi controller to the physical engraver. 10-byte binary
//     command frames in 80-command batches, with status-byte handshake. See
//     docs/architecture/v1-engrave-spec.md for the full audit.
//
//   - wire/sh1e/: the QR-transport envelope (magic + version + length +
//     CRC32 + CBOR-encoded design). See docs/architecture/sh1e-spec.md.
//
// SH1E is a transport from composer to Pi; the live wire is the protocol
// the Pi speaks to the engraver. They are NOT the same and don't share an
// encoder.
//
// Status: STUB — live wire encoder to be lifted from upstream's mjolnir/
// driver at v1.3.0.
package wire
