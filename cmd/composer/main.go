//go:build js && wasm

// Command composer is the browser-side plate composer for SeedHammer v1.
//
// Compiles to WebAssembly via:
//
//	GOOS=js GOARCH=wasm go build -o ./web/composer/composer.wasm ./cmd/composer
//
// The static shell (HTML/CSS/JS) lives under web/composer/. Exported JS
// surface is documented in web/composer/app.js — every JS function listed
// there is bound here.
//
// This is the Phase 1 milestone build: a minimal composer that proves the
// Go-to-WASM pipeline works and produces canonical SH1E payloads. The
// preview/QR/SVG-mode features land in follow-up commits.
package main

import (
	"fmt"
	"syscall/js"

	"github.com/mineracks/seedhammer-v1-companion/engrave/wire/sh1e"
)

const composerVersion = "v0.1-phase1-milestone"

func main() {
	js.Global().Set("composerVersion", js.FuncOf(exportVersion))
	js.Global().Set("composerPlateTypes", js.FuncOf(exportPlateTypes))
	js.Global().Set("composerEncodeText", js.FuncOf(exportEncodeText))
	// Block forever so the Go runtime keeps the exported funcs alive.
	select {}
}

// exportVersion returns the composer version string. Used by the JS shell
// to show what's loaded.
//
//	JS signature: composerVersion() -> string
func exportVersion(this js.Value, args []js.Value) any {
	return composerVersion
}

// exportPlateTypes returns the supported v1 plate types as a JS array of
// {id, name, w_mm, h_mm} objects. Used to populate the plate picker.
//
//	JS signature: composerPlateTypes() -> Array<{id, name, w_mm, h_mm}>
//
// Values mirror upstream backup/backup.go at v1.3.0.
func exportPlateTypes(this js.Value, args []js.Value) any {
	return js.ValueOf([]any{
		map[string]any{"id": int(sh1e.SmallPlate), "name": "Small", "w_mm": 85, "h_mm": 55},
		map[string]any{"id": int(sh1e.SquarePlate), "name": "Square", "w_mm": 85, "h_mm": 85},
		map[string]any{"id": int(sh1e.LargePlate), "name": "Large", "w_mm": 85, "h_mm": 134},
	})
}

// exportEncodeText takes a plate type (number) and a JS array of line
// strings, produces an SH1E envelope.
//
//	JS signature: composerEncodeText(plateType:number, lines:string[]) -> Uint8Array
//
// Throws on any validation failure (bad plate type, non-ASCII text, too
// many lines, …) — JS sees these as caught exceptions with a readable
// message.
func exportEncodeText(this js.Value, args []js.Value) any {
	if len(args) != 2 {
		return jsError(fmt.Errorf("composerEncodeText: expected 2 args, got %d", len(args)))
	}
	plateType := sh1e.PlateType(args[0].Int())

	// Convert JS array of strings to []string.
	jsLines := args[1]
	n := jsLines.Length()
	lines := make([]string, 0, n)
	for i := 0; i < n; i++ {
		s := jsLines.Index(i).String()
		if s == "" {
			continue
		}
		lines = append(lines, s)
	}

	// For Phase 1 milestone: each non-empty line becomes one TextBlock
	// at FontComfortaa size 12, stacked at y = 5 + i*8 mm, x = 5 mm,
	// left-aligned. Layout polish lands in a follow-up commit.
	const (
		fontSize   = 12
		xMM        = 5
		yStartMM   = 5
		yStrideMM  = 8
		alignment  = sh1e.AlignLeft
		fontFamily = sh1e.FontComfortaa
	)
	blocks := make([]sh1e.TextBlock, 0, len(lines))
	for i, line := range lines {
		blocks = append(blocks, sh1e.TextBlock{
			FontID:    fontFamily,
			Size:      fontSize,
			XMM:       xMM,
			YMM:       int16(yStartMM + i*yStrideMM),
			Alignment: alignment,
			Text:      line,
		})
	}

	design := sh1e.Design{
		PlateType:  plateType,
		TextBlocks: blocks,
	}
	bytes, err := sh1e.Encode(design)
	if err != nil {
		return jsError(err)
	}
	return uint8Array(bytes)
}

// jsError wraps a Go error as a JS-side thrown Error. syscall/js doesn't
// expose Throw directly from FuncOf callbacks — returning a JS Error
// object causes the caller's try/catch to see it; alternative is to
// panic, but that abuses the runtime for control flow.
func jsError(err error) any {
	jsErr := js.Global().Get("Error").New(err.Error())
	js.Global().Get("console").Call("error", jsErr)
	panic(jsErr) // caught as a Go panic across the JS boundary, surfaces as a JS exception
}

// uint8Array copies a Go []byte into a fresh JS Uint8Array. Required
// because js.CopyBytesToJS is the only safe way to transfer a Go byte
// slice to JS — direct js.ValueOf on []byte doesn't produce a typed array.
func uint8Array(src []byte) js.Value {
	dst := js.Global().Get("Uint8Array").New(len(src))
	js.CopyBytesToJS(dst, src)
	return dst
}
