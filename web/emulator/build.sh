#!/usr/bin/env bash
# Build the SeedHammer v1 emulator WASM bundle.
# wasm_exec.js is shared with the composer at ../composer/wasm_exec.js.
set -euo pipefail

cd "$(dirname "$0")"
REPO_ROOT=$(cd ../.. && pwd)

cd "$REPO_ROOT"
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" \
  -o ./web/emulator/emulator.wasm \
  ./cmd/emulator

size=$(stat -f%z ./web/emulator/emulator.wasm 2>/dev/null || stat -c%s ./web/emulator/emulator.wasm)
echo "built: ./web/emulator/emulator.wasm (${size} bytes)"
echo "serve: python3 -m http.server -d ./web 38080"
echo "  →    open http://localhost:38080/emulator/"
