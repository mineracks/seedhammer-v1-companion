//go:build !js || !wasm

// Host-build placeholder for cmd/emulator. The emulator is WASM-only.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "emulator: this binary is WebAssembly-only.")
	fmt.Fprintln(os.Stderr, "Build it with:")
	fmt.Fprintln(os.Stderr, "  GOOS=js GOARCH=wasm go build -o ./web/emulator/emulator.wasm ./cmd/emulator")
	os.Exit(2)
}
