//go:build js && wasm

// Command emulator is a browser-based SeedHammer v1 firmware runner.
//
// Phase 2 scaffolding stage. This binary boots a 240×240 canvas-backed
// LCD mock and an 8-button input layer that maps keyboard events to the
// v1's joystick + 3 keys. The real firmware GUI lift (upstream's gui/
// package + its assets/layout/op/saver/text/widget subpackages) lands in
// a follow-up commit; today this stage proves the build pipeline + the
// platform/v1.Platform interface contract.
//
// Build:
//
//	GOOS=js GOARCH=wasm go build -o ./web/emulator/emulator.wasm ./cmd/emulator
//
// The static shell (HTML/CSS/JS) lives under web/emulator/. JS exports
// documented in web/emulator/app.js.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"syscall/js"

	"github.com/mineracks/seedhammer-v1-companion/platform/v1"
)

const emulatorVersion = "v0.1-phase2-stub"

// v1 hardware native LCD resolution.
const (
	lcdWidth  = 240
	lcdHeight = 240
)

// browserPlatform implements v1.Platform against the JS host.
//
// Display() draws into an *image.RGBA buffer then ships it to JS via a
// Uint8ClampedArray that the JS shell paints onto a <canvas>. Events()
// returns a channel fed by exportPushEvent calls from JS.
type browserPlatform struct {
	frame  *image.RGBA
	events chan v1.Event
}

func newBrowserPlatform() *browserPlatform {
	return &browserPlatform{
		frame:  image.NewRGBA(image.Rect(0, 0, lcdWidth, lcdHeight)),
		events: make(chan v1.Event, 32),
	}
}

func (p *browserPlatform) Events() <-chan v1.Event { return p.events }

func (p *browserPlatform) Display(frame image.Image) {
	draw.Draw(p.frame, p.frame.Bounds(), frame, frame.Bounds().Min, draw.Src)
	// Convert RGBA buffer → JS Uint8ClampedArray and call back into JS.
	jsBuf := js.Global().Get("Uint8ClampedArray").New(len(p.frame.Pix))
	js.CopyBytesToJS(jsBuf, p.frame.Pix)
	js.Global().Call("emulatorPaint", jsBuf, lcdWidth, lcdHeight)
}

// Push an event from JS into the events channel.
func (p *browserPlatform) push(button v1.Button, pressed bool) {
	select {
	case p.events <- v1.Event{Button: button, Pressed: pressed}:
	default:
		// Drop on full — shouldn't happen with the modest buffer the
		// firmware needs, but better than blocking the JS thread.
	}
}

var plat *browserPlatform

func main() {
	plat = newBrowserPlatform()

	js.Global().Set("emulatorVersion", js.FuncOf(exportVersion))
	js.Global().Set("emulatorPushEvent", js.FuncOf(exportPushEvent))
	js.Global().Set("emulatorBootScreen", js.FuncOf(exportBootScreen))
	js.Global().Set("emulatorLCDSize", js.ValueOf(map[string]any{
		"w": lcdWidth,
		"h": lcdHeight,
	}))

	// Show the placeholder boot screen so the canvas isn't blank on load.
	drawBootScreen(plat.frame)
	plat.Display(plat.frame)

	// Future: wire plat to gui.Loop or whatever the lifted GUI exposes.
	// For now, just consume events so the channel doesn't fill up.
	go func() {
		for ev := range plat.events {
			// Echo to console for debugging; the real GUI will replace this.
			js.Global().Get("console").Call("log",
				fmt.Sprintf("emu: %s %s",
					buttonName(ev.Button),
					boolStr(ev.Pressed, "press", "release"),
				),
			)
		}
	}()

	select {} // keep the runtime alive
}

func exportVersion(this js.Value, args []js.Value) any {
	return emulatorVersion
}

// exportPushEvent: emulatorPushEvent(buttonId:number, pressed:boolean)
func exportPushEvent(this js.Value, args []js.Value) any {
	if len(args) != 2 {
		return nil
	}
	id := args[0].Int()
	pressed := args[1].Bool()
	if id < 0 || id > int(v1.Button3) {
		return nil
	}
	plat.push(v1.Button(id), pressed)
	return nil
}

// exportBootScreen redraws the boot placeholder, useful for re-testing
// after manual mucking.
func exportBootScreen(this js.Value, args []js.Value) any {
	drawBootScreen(plat.frame)
	plat.Display(plat.frame)
	return nil
}

// drawBootScreen fills the frame with a minimal welcome image so users see
// SOMETHING the moment the WASM finishes loading. Future: this is replaced
// by gui.Loop()'s first frame.
func drawBootScreen(dst *image.RGBA) {
	// Background.
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.RGBA{0x12, 0x12, 0x12, 0xff}}, image.Point{}, draw.Src)

	// Frame border to make the LCD area unambiguous.
	border := color.RGBA{0xff, 0x88, 0x00, 0xff} // accent orange
	for x := 0; x < lcdWidth; x++ {
		dst.SetRGBA(x, 0, border)
		dst.SetRGBA(x, lcdHeight-1, border)
	}
	for y := 0; y < lcdHeight; y++ {
		dst.SetRGBA(0, y, border)
		dst.SetRGBA(lcdWidth-1, y, border)
	}

	// Eight tick marks around the perimeter to show the button positions.
	// Just a visual cue that this is real hardware-resolution rendering.
	tick := color.RGBA{0xaa, 0xaa, 0xaa, 0xff}
	for i := 0; i < 16; i++ {
		dst.SetRGBA(lcdWidth/2-1+i-8, lcdHeight/2, tick)
		dst.SetRGBA(lcdWidth/2, lcdHeight/2-1+i-8, tick)
	}
}

func buttonName(b v1.Button) string {
	switch b {
	case v1.ButtonUp:
		return "Up"
	case v1.ButtonDown:
		return "Down"
	case v1.ButtonLeft:
		return "Left"
	case v1.ButtonRight:
		return "Right"
	case v1.ButtonCenter:
		return "Center"
	case v1.Button1:
		return "Button1"
	case v1.Button2:
		return "Button2"
	case v1.Button3:
		return "Button3"
	}
	return "btn-" + strconv.Itoa(int(b))
}

func boolStr(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}
