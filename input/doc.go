// Package input is the v1 controller's hardware input abstraction.
//
// On a real Pi: reads GPIO pins from the WaveShare 1.3" LCD HAT via
// periph.io (driver/wshat in upstream layout).
//
// In the browser emulator: keyboard event source from the host page,
// translated to the same Event{Button, Pressed} envelope.
//
// Eight inputs total: 5-way joystick (Up/Down/Left/Right/Center) + 3 keys
// (Button1/Button2/Button3). GPIO pin map (BCM283x):
//
//	Up      → GPIO 6
//	Down    → GPIO 19
//	Left    → GPIO 5
//	Right   → GPIO 26
//	Center  → GPIO 13
//	Button1 → GPIO 21
//	Button2 → GPIO 20
//	Button3 → GPIO 16
//
// Active-low with internal pull-ups, 10 ms debounce.
//
// Status: STUB — to be lifted from upstream at v1.3.0 (driver/wshat path,
// formerly input/input.go pre the v1.2.0 refactor).
package input
