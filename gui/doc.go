// Package gui is the v1 controller's on-device UI — screen drawing, menu
// navigation, page state machine.
//
// Self-contained, hardware-abstracted: it talks to platform/ for input
// events and frame output. That abstraction lets the same gui run on a
// real Pi (via driver/wshat + driver/drm) and in the browser emulator
// (via platform/v1 + canvas).
//
// Screen flow follows the v1 controller's existing conventions:
//
//   - Joystick (Up/Down/Left/Right/Center) = navigation
//   - Button1 = back
//   - Button2 = secondary action (hold-to-arm dry-run)
//   - Button3 = primary confirm (hold-to-engrave)
//
// Hold-to-confirm distinguishes Pressed from Click with a confirmDelay
// timer (see gui.go in upstream).
//
// Status: STUB — to be lifted from upstream seedhammer/seedhammer at v1.3.0.
// Browser-emulator-specific tweaks (mock camera, keyboard event source)
// live in platform/v1, NOT in this package.
package gui
