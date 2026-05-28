//go:build !js || !wasm

// This file is the host-build placeholder for cmd/composer.
//
// The composer is a WebAssembly module — there's no useful host binary.
// We keep one tiny main() here so `go build ./...` doesn't emit
// "build constraints exclude all Go files" for this package on non-WASM
// targets, which would noise up CI and IDE output.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "composer: this binary is WebAssembly-only.")
	fmt.Fprintln(os.Stderr, "Build it with:")
	fmt.Fprintln(os.Stderr, "  GOOS=js GOARCH=wasm go build -o ./web/composer/composer.wasm ./cmd/composer")
	os.Exit(2)
}
