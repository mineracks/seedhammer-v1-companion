// Package sh1e implements the v1 SH1E plate-design envelope.
//
// SH1E ships a plate design (plate type + text blocks + optional SVG paths)
// from the browser-side composer to a SeedHammer v1 Pi controller, intended
// primarily for transport via QR code(s) scanned by the Pi's camera.
//
// The envelope structure:
//
//	+----------+----------+--------------+--------+-----------+
//	| magic    | version  | payload_len  | crc32  | payload   |
//	| 4 bytes  | 1 byte   | 2 bytes (LE) | 4 bytes| N bytes   |
//	| "SH1E"   |   0x01   |  uint16      |        |  CBOR     |
//	+----------+----------+--------------+--------+-----------+
//
// Payload is CBOR-encoded per RFC 8949 deterministic encoding rules so a
// given Design produces byte-identical bytes every time.
//
// The composer side encodes; the Pi controller side decodes + validates +
// rasterises locally using the trusted engrave/ pipeline. The composer
// never ships pre-rendered engraver commands — keeping the security
// boundary on the Pi.
//
// Full spec: docs/architecture/sh1e-spec.md
//
// Status: STUB — reference encoder + decoder + canonical-encoding test
// fixtures pending. Will land in Phase 1 of the project.
package sh1e
