#!/usr/bin/env bash
# Build the SeedHammer v1 composer WASM bundle and stage wasm_exec.js.
#
# Output: ./composer.wasm + ./wasm_exec.js alongside index.html. Open
# index.html via any static-file server (e.g. `python3 -m http.server`)
# to load the composer in a browser.
set -euo pipefail

cd "$(dirname "$0")"
REPO_ROOT=$(cd ../.. && pwd)

GOROOT=$(go env GOROOT)
install -m 644 "$GOROOT/lib/wasm/wasm_exec.js" ./wasm_exec.js

cd "$REPO_ROOT"
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" \
  -o ./web/composer/composer.wasm \
  ./cmd/composer

size=$(stat -f%z ./web/composer/composer.wasm 2>/dev/null || stat -c%s ./web/composer/composer.wasm)
echo "built: ./web/composer/composer.wasm (${size} bytes)"
echo "serve: python3 -m http.server -d ./web/composer 38080"
echo "  →    open http://localhost:38080"
