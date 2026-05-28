// Command emulator runs the SeedHammer v1 controller firmware in a browser.
//
// Compiles to WebAssembly via GOOS=js GOARCH=wasm. The HTML/CSS shell that
// hosts this WASM lives under web/emulator/.
//
// The WASM hosts the real v1 gui/, input/, and engrave/ packages, with
// hardware-side drivers (driver/wshat for buttons, driver/drm for the LCD,
// driver/libcamera for the camera, driver/mjolnir for the engraver)
// replaced by browser-side mocks:
//
//   - Buttons → keyboard events (arrows / Enter / 1, 2, 3)
//   - LCD → HTML5 canvas
//   - Camera → mock that reads QRs from sibling panes on the same page
//   - Engraver → null sink (preview mode) or visual playback animation
//
// Status: STUB — implementation will lift upstream v1.3.0 gui/ and input/
// packages, with browser-side platform adapters in platform/v1/.
//
// Modelled architecturally on Gangleri42's cmd/wasmemu at:
// https://github.com/Gangleri42/seedhammer/tree/seedhammer-features/cmd/wasmemu
// (pinned at 0a3c63efb125d17d8ec86ce739ecd058c8747cfe), but his wasmemu
// emulates SH-II's touch UI — v1 is button-driven, so we rebuild from scratch
// while keeping his PWA shell pattern.
package main
