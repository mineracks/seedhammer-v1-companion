// Package mjolnir drives the MarkingWay engraving machine over USB serial.
//
// "Mjolnir" because it talks to the hammer. The wire protocol is documented
// in docs/architecture/v1-engrave-spec.md.
//
// LIFTED from upstream seedhammer/seedhammer at v1.3.0
// (commit 2f071c1d8f23eb7fd39b15fc0acb8874113f801e) path driver/mjolnir/.
// On a real device this opens /dev/ttyUSB0 via github.com/tarm/serial; in
// the browser emulator the open call is intercepted by platform/v1's
// browser adapter and routed to a null sink (preview) or animation harness
// (visual playback).
package mjolnir
