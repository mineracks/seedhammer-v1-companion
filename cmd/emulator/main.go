//go:build js && wasm

// Command emulator is a browser-based SeedHammer v1 firmware runner.
//
// Loads the upstream v1.3.0 gui package and drives it through a
// browser-side Platform implementation:
//   - Display: a 240×240 canvas, painted from Go RGBA via JS callback
//   - Input: keyboard + on-screen button events through gui.ButtonEvent
//   - Engraver: a no-op stub (the browser doesn't drive real hardware)
//   - Camera: QR handoff injection. emulatorInjectQR(payload, rgba, w, h)
//     stores a decoded QR payload + preview frame; CameraFrame() then
//     emits the preview as a FrameEvent and ScanQR() returns the payload
//     (one-shot), so the firmware's own decoder chain (ur / nonstandard /
//     seedqr) consumes it exactly as if the camera had seen the QR.
//     Without an injection pending, the camera stays in its stubbed
//     "no camera" state.
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

const emulatorVersion = "v0.3-phase2.5-handoff"

const (
	lcdWidth  = 240
	lcdHeight = 240
)

// browserPlatform implements gui.Platform against the JS host.
type browserPlatform struct {
	frame  *image.RGBA
	events chan v1.Event
	// wake is a 1-buffered channel that the select in Events() reads
	// from. exportSetSDCard, exportCameraFrame (future), and any other
	// non-button event source pokes wake after appending to pending so
	// the Events() wait returns immediately and drains the new event.
	wake chan struct{}

	mu        sync.Mutex
	pending   []gui.Event
	dirtyRect image.Rectangle
	chunkSent bool

	// QR handoff injection (emulatorInjectQR). injPayload is returned by
	// the next ScanQR call; injFrame is the camera-preview image shown
	// while the injection is pending.
	injPayload []byte
	injFrame   *image.YCbCr
}

func newBrowserPlatform() *browserPlatform {
	return &browserPlatform{
		frame:  image.NewRGBA(image.Rect(0, 0, lcdWidth, lcdHeight)),
		events: make(chan v1.Event, 64),
		wake:   make(chan struct{}, 1),
	}
}

func (p *browserPlatform) signalWake() {
	select {
	case p.wake <- struct{}{}:
	default:
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
	case <-p.wake:
		// Non-button event arrived (e.g. SDCardEvent). Fall through to
		// the pending drain at the bottom.
	case <-timer.C:
	}
	// Drain any extras (button events) that piled up while we waited.
	for {
		select {
		case ev := <-p.events:
			out = append(out, p.toGuiEvent(ev))
		default:
			// Also re-drain pending — if signalWake fired, the SDCard
			// or other event is sitting there now.
			out = append(out, p.drainPending()...)
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
	p.signalWake()
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.injPayload != nil {
		// Handoff pending: show the injected preview. The gui only reads
		// the Y plane (it grayscales the feed), so injFrame is YCbCr with
		// luminance filled and neutral chroma.
		p.pending = append(p.pending, gui.FrameEvent{Image: p.injFrame}.Event())
		return
	}
	// Stub: no live camera in the browser. Emit an error FrameEvent so
	// the gui's QR-scan screen shows its "no camera" state instead of
	// waiting forever.
	p.pending = append(p.pending, gui.FrameEvent{Error: errCameraStubbed}.Event())
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
	// One-shot: hand the injected payload to the firmware's decoder
	// chain (ur / nonstandard / seedqr). The qr image is ignored — JS
	// already decoded the pixels (jsQR); the payload re-enters the same
	// parse path a real camera decode would.
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.injPayload == nil {
		return nil, nil
	}
	res := [][]byte{p.injPayload}
	p.injPayload = nil
	p.injFrame = nil
	return res, nil
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
	js.Global().Set("emulatorSetSDCard", js.FuncOf(exportSetSDCard))
	js.Global().Set("emulatorInjectQR", js.FuncOf(exportInjectQR))
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

// exportSetSDCard: emulatorSetSDCard(inserted:boolean)
//
// Synthesizes a gui.SDCardEvent so the firmware sees the same insert /
// remove notification it would get from the kernel's hotplug events on
// real hardware. Lets users trigger the "REMOVE SD CARD" screen from
// the browser without dealing with the actual kernel events.
func exportSetSDCard(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return nil
	}
	inserted := args[0].Bool()
	plat.mu.Lock()
	plat.pending = append(plat.pending, gui.SDCardEvent{Inserted: inserted}.Event())
	plat.mu.Unlock()
	plat.signalWake() // unblock any in-flight Events() wait
	return nil
}

// exportInjectQR: emulatorInjectQR(payload:Uint8Array, rgba:Uint8ClampedArray|null, w:int, h:int)
//
// QR handoff from the SeedSigner sim (or any QR source). payload is the
// decoded QR contents; rgba is an optional camera-preview image of the QR
// as the user saw it. The firmware consumes the payload via ScanQR when
// the user navigates to a scan screen.
func exportInjectQR(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return nil
	}
	payload := make([]byte, args[0].Length())
	js.CopyBytesToGo(payload, args[0])
	if len(payload) == 0 {
		return nil
	}

	// Build the YCbCr preview the gui expects (it only reads the Y plane).
	w, h := 8, 8
	var rgba []byte
	if len(args) >= 4 && !args[1].IsNull() && !args[1].IsUndefined() {
		w, h = args[2].Int(), args[3].Int()
		rgba = make([]byte, args[1].Length())
		js.CopyBytesToGo(rgba, args[1])
	}
	frame := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for i := range frame.Cb {
		frame.Cb[i], frame.Cr[i] = 128, 128
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			luma := byte(0xff) // white frame when no preview supplied
			if rgba != nil {
				o := (y*w + x) * 4
				// integer BT.601 luma
				luma = byte((299*int(rgba[o]) + 587*int(rgba[o+1]) + 114*int(rgba[o+2])) / 1000)
			}
			frame.Y[y*frame.YStride+x] = luma
		}
	}

	plat.mu.Lock()
	plat.injPayload = payload
	plat.injFrame = frame
	plat.mu.Unlock()
	plat.signalWake()
	return nil
}

func clearBlack(dst *image.RGBA) {
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.RGBA{0, 0, 0, 0xff}}, image.Point{}, draw.Src)
}
