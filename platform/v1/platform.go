// Package v1 defines the platform adapter layer that v1's gui/, input/,
// and engrave/ packages bind against.
//
// Two build-time targets implement Platform:
//
//   - Real device (GOOS=linux GOARCH=arm on Pi Zero): driver/drm for
//     LCD frame output, driver/wshat for GPIO buttons, driver/libcamera
//     for the QR-scanner camera, driver/mjolnir for the engraver USB
//     serial.
//
//   - Browser emulator (GOOS=js GOARCH=wasm): browser canvas writer,
//     keyboard-event mapper, mock camera reading from a sibling pane's
//     canvas, and a null engrave sink (or a visual playback harness).
//
// The interface lives here (not in gui/) so we can swap backends without
// touching the GUI code. gui.Context takes a Platform value and never
// calls any driver/ package directly.
package v1

import (
	"image"

	"github.com/mineracks/seedhammer-v1-companion/font/constant"
)

// Button identifies one of the eight physical inputs on the v1 hardware.
type Button int

const (
	ButtonUp Button = iota
	ButtonDown
	ButtonLeft
	ButtonRight
	ButtonCenter
	Button1
	Button2
	Button3
)

// Event is what a Platform delivers to the GUI's input loop.
type Event struct {
	Button  Button
	Pressed bool // true = press, false = release
}

// Platform is the contract every backend implements. The GUI receives a
// Platform value at startup and drives every external interaction through
// it. Adding a new backend means implementing this interface — no GUI code
// changes.
type Platform interface {
	// Events returns the channel of input events. Closed when the
	// platform is shutting down.
	Events() <-chan Event

	// Display writes a frame to the LCD-equivalent. The image is the
	// full screen at the platform's native resolution (240×240 for v1).
	Display(frame image.Image)

	// EngraveFont returns the vector engraving face. Both real and
	// emulator backends return the same data — it's bundled with the
	// firmware.
	EngraveFont() *constant.Face
}

// constant.Face is the type alias we use in the public surface, re-exported
// here so callers don't have to import font/vector directly.
// (Today this is just *vector.Face under the hood; the alias lets us swap
// implementations without rippling change through the GUI.)
type Face = constant.Face
