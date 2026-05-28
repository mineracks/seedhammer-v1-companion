//go:build js && wasm

// Command emulator is a browser-based SeedHammer v1 firmware runner.
//
// Loads the upstream v1.3.0 gui package and drives it through a
// browser-side Platform implementation:
//   - Display: a 240×240 canvas, painted from Go RGBA via JS callback
//   - Input: keyboard + on-screen button events through gui.ButtonEvent
//   - Engraver: a no-op stub (the browser doesn't drive real hardware)
//   - Camera: stub that emits empty FrameEvents (real QR-scan handoff
//     lands once the SeedSigner sim wiring lands in Phase 2.5)
//
// Build:
//
//	GOOS=js GOARCH=wasm go build -o ./web/emulator/emulator.wasm ./cmd/emulator
package main

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"sync"
	"syscall/js"
	"time"

	"github.com/mineracks/seedhammer-v1-companion/backup"
	"github.com/mineracks/seedhammer-v1-companion/engrave"
	"github.com/mineracks/seedhammer-v1-companion/gui"
	v1 "github.com/mineracks/seedhammer-v1-companion/platform/v1"
)

const emulatorVersion = "v0.2-phase2-gui"

const (
	lcdWidth  = 240
	lcdHeight = 240
)

// browserPlatform implements gui.Platform against the JS host.
type browserPlatform struct {
	frame  *image.RGBA
	events chan v1.Event

	mu        sync.Mutex
	pending   []gui.Event
	dirtyRect image.Rectangle
	chunkSent bool
}

func newBrowserPlatform() *browserPlatform {
	return &browserPlatform{
		frame:  image.NewRGBA(image.Rect(0, 0, lcdWidth, lcdHeight)),
		events: make(chan v1.Event, 64),
	}
}

// ─── gui.Platform impl ────────────────────────────────────────────────────

func (p *browserPlatform) Events(deadline time.Time) []gui.Event {
	// Drain the v1.Event channel into gui.ButtonEvents. If no events
	// pending, block (briefly) waiting for one or until deadline.
	wait := time.Until(deadline)
	if wait < 0 {
		wait = 0
	}
	out := p.drainPending()
	if len(out) > 0 {
		return out
	}
	if wait == 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case ev := <-p.events:
		out = append(out, p.toGuiEvent(ev))
	case <-timer.C:
	}
	// Drain any extras that piled up while we were waiting.
	for {
		select {
		case ev := <-p.events:
			out = append(out, p.toGuiEvent(ev))
		default:
			return out
		}
	}
}

func (p *browserPlatform) drainPending() []gui.Event {
	p.mu.Lock()
	out := p.pending
	p.pending = nil
	p.mu.Unlock()
	return out
}

func (p *browserPlatform) toGuiEvent(ev v1.Event) gui.Event {
	return gui.ButtonEvent{
		Button:  gui.Button(ev.Button), // enum order matches by construction
		Pressed: ev.Pressed,
	}.Event()
}

// push is called from the JS bridge — feeds the buffered channel.
func (p *browserPlatform) push(button v1.Button, pressed bool) {
	select {
	case p.events <- v1.Event{Button: button, Pressed: pressed}:
	default:
	}
}

func (p *browserPlatform) Wakeup() {
	// no-op — JS-driven runtime; nothing to wake from.
}

func (p *browserPlatform) PlateSizes() []backup.PlateSize {
	// Mirror what's defined in backup.PlateSize. v1.3.0 ships
	// SquarePlate and LargePlate.
	return []backup.PlateSize{backup.SquarePlate, backup.LargePlate}
}

func (p *browserPlatform) Engraver() (gui.Engraver, error) {
	// The browser can't engrave anything. Return an Engraver that
	// politely says "no" if the GUI ever tries to drive it.
	return nullEngraver{}, nil
}

func (p *browserPlatform) EngraverParams() engrave.Params {
	// Values copied from upstream driver/mjolnir.Params at v1.3.0.
	// We can't import the mjolnir package here because it transitively
	// pulls in tarm/serial, which doesn't compile to GOOS=js (uses
	// OS-specific syscalls). The layout math doesn't need the serial
	// driver — just these two constants.
	return engrave.Params{
		StrokeWidth: 38,
		Millimeter:  126,
	}
}

func (p *browserPlatform) CameraFrame(size image.Point) {
	// Stub: no camera in the browser yet. Emit an error FrameEvent so
	// the gui's QR-scan screen stays in its "no camera" state instead
	// of waiting forever.
	p.mu.Lock()
	p.pending = append(p.pending, gui.FrameEvent{Error: errCameraStubbed}.Event())
	p.mu.Unlock()
}

func (p *browserPlatform) Now() time.Time { return time.Now() }

func (p *browserPlatform) DisplaySize() image.Point {
	return image.Pt(lcdWidth, lcdHeight)
}

func (p *browserPlatform) Dirty(r image.Rectangle) error {
	p.mu.Lock()
	p.dirtyRect = r.Intersect(p.frame.Bounds())
	p.chunkSent = false
	p.mu.Unlock()
	return nil
}

func (p *browserPlatform) NextChunk() (draw.RGBA64Image, bool) {
	p.mu.Lock()
	if p.chunkSent || p.dirtyRect.Empty() {
		p.mu.Unlock()
		// One-chunk model: we ship the whole frame to JS in a single
		// JS callback when the gui completes a Dirty/NextChunk cycle.
		p.flushFrame()
		return nil, false
	}
	p.chunkSent = true
	r := p.dirtyRect
	p.mu.Unlock()
	// gui writes into the returned RGBA64Image — sub-image of our frame
	// for the dirty rect. Our buffer is RGBA which satisfies
	// draw.RGBA64Image via the standard image package.
	return p.frame.SubImage(r).(*image.RGBA), true
}

// flushFrame ships the current frame buffer to JS. Called after each
// Dirty/NextChunk render cycle.
func (p *browserPlatform) flushFrame() {
	jsBuf := js.Global().Get("Uint8ClampedArray").New(len(p.frame.Pix))
	js.CopyBytesToJS(jsBuf, p.frame.Pix)
	js.Global().Call("emulatorPaint", jsBuf, lcdWidth, lcdHeight)
}

func (p *browserPlatform) ScanQR(qr *image.Gray) ([][]byte, error) {
	// Stub: no decodes. Real implementation lands when SeedSigner sim
	// handoff wires up — the mock camera reads a sibling pane's canvas.
	return nil, nil
}

func (p *browserPlatform) Debug() bool { return false }

var errCameraStubbed = errors.New("camera not implemented in browser stub")

// ─── nullEngraver ─────────────────────────────────────────────────────────

type nullEngraver struct{}

func (nullEngraver) Engrave(_ backup.PlateSize, _ engrave.Plan, _ <-chan struct{}) error {
	return errors.New("engraver not connected (browser emulator)")
}
func (nullEngraver) Close() {}

// ─── JS bridge ────────────────────────────────────────────────────────────

var plat *browserPlatform

func main() {
	plat = newBrowserPlatform()

	// Initial paint so the canvas isn't blank during gui bring-up.
	clearBlack(plat.frame)
	plat.flushFrame()

	js.Global().Set("emulatorVersion", js.FuncOf(exportVersion))
	js.Global().Set("emulatorPushEvent", js.FuncOf(exportPushEvent))
	js.Global().Set("emulatorLCDSize", js.ValueOf(map[string]any{
		"w": lcdWidth, "h": lcdHeight,
	}))

	app, err := gui.NewApp(plat, emulatorVersion)
	if err != nil {
		js.Global().Get("console").Call("error", "gui.NewApp failed: "+err.Error())
		select {}
	}

	// Drive frames in a goroutine. Each Frame call processes events
	// and may render a new frame via Dirty + NextChunk.
	go func() {
		for {
			app.Frame()
		}
	}()

	select {}
}

func exportVersion(this js.Value, args []js.Value) any {
	return emulatorVersion
}

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

func clearBlack(dst *image.RGBA) {
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.RGBA{0, 0, 0, 0xff}}, image.Point{}, draw.Src)
}
