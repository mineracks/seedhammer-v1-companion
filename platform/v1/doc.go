// Package v1 holds platform adapters that bind gui/, input/, and engrave/
// to either a real SeedHammer v1 device or the browser emulator.
//
// Two build-time targets:
//
//   - real:    GOOS=linux GOARCH=arm — for the Pi Zero v1.3 itself.
//              Binds gui/ frame output to driver/drm/, input/ to
//              driver/wshat/ (GPIO via periph.io), engrave/ to
//              driver/mjolnir/ (USB-serial to MarkingWay).
//
//   - browser: GOOS=js GOARCH=wasm — for the in-browser emulator.
//              Binds gui/ frame output to a JS-exposed canvas writer,
//              input/ to keyboard events forwarded from the page,
//              engrave/ to a null sink (preview) or animation harness
//              (visual playback).
//
// Build constraints select the right adapter; both expose the same
// platform.Driver interface so gui/ doesn't know which it's running on.
//
// Status: STUB — interface design and both adapters land in Phase 1 + Phase 2.
package v1
