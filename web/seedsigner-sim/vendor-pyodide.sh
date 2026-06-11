#!/usr/bin/env bash
# Vendor the Pyodide runtime + the wheels the sim loads (pillow, numpy) +
# jsQR into vendor/, so the sim runs fully offline. worker.js prefers
# vendor/pyodide/ and falls back to the CDN when this hasn't been run.
#
# Version must match PYODIDE_VERSION in worker.js.
set -euo pipefail

PYODIDE_VERSION=0.27.2
CDN="https://cdn.jsdelivr.net/pyodide/v${PYODIDE_VERSION}/full"
JSQR_URL="https://cdn.jsdelivr.net/npm/jsqr@1.4.0/dist/jsQR.js"

cd "$(dirname "$0")"
OUT=vendor/pyodide
mkdir -p "$OUT"

fetch() { # fetch <filename>
  echo "  $1"
  curl -fsSL -o "$OUT/$1" "$CDN/$1"
}

echo "==> pyodide ${PYODIDE_VERSION} runtime"
for f in pyodide.js pyodide.asm.js pyodide.asm.wasm python_stdlib.zip pyodide-lock.json; do
  fetch "$f"
done

echo "==> wheels for: pillow numpy hashlib (+ transitive deps from the lockfile)"
python3 - "$OUT/pyodide-lock.json" <<'EOF' | while read -r f; do fetch "$f"; done
import json, sys

lock = json.load(open(sys.argv[1]))
pkgs = lock["packages"]
want, files = ["pillow", "numpy", "hashlib"], set()
while want:
    name = want.pop()
    p = pkgs[name.lower()]
    if p["file_name"] in files:
        continue
    files.add(p["file_name"])
    want.extend(p.get("depends", []))
print("\n".join(sorted(files)))
EOF

echo "==> jsQR"
curl -fsSL -o vendor/jsqr.js "$JSQR_URL"

du -sh vendor/pyodide vendor/jsqr.js
echo "done — worker.js will now load locally."
