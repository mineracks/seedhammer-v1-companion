#!/usr/bin/env bash
# Build the SeedSigner sim's vendor bundle: upstream Python source at the
# pinned baseline SHA + pure-Python deps, zipped for Pyodide's unpackArchive.
#
# Pin lives in docs/architecture/BASELINES.md ("SeedSigner upstream" section).
# To bump: update BASELINES.md (append a dated section), then update
# SEEDSIGNER_SHA here.
set -euo pipefail

SEEDSIGNER_SHA=e0a80d4b33b8eb7fb1e9fd14a27b7cd11c7d2cd6
SEEDSIGNER_REPO=https://git.mineracks.com/seedsigner/seedsigner.git

# Pinned per upstream requirements.txt at the baseline SHA (pure Python only;
# pyzbar is C and is mocked in boot.py).
PYTHON_DEPS=(embit==0.8.0 qrcode==7.3.1 urtypes==1.0.1)

cd "$(dirname "$0")"
OUT=vendor
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

echo "==> fetching SeedSigner @ ${SEEDSIGNER_SHA}"
git init -q "$STAGE/seedsigner"
git -C "$STAGE/seedsigner" remote add origin "$SEEDSIGNER_REPO"
if ! git -C "$STAGE/seedsigner" fetch -q --depth 1 origin "$SEEDSIGNER_SHA" 2>/dev/null; then
  # Mirror may not allow fetch-by-SHA; fall back to dev branch and verify.
  git -C "$STAGE/seedsigner" fetch -q --depth 1 origin dev
  head=$(git -C "$STAGE/seedsigner" rev-parse FETCH_HEAD)
  if [[ "$head" != "$SEEDSIGNER_SHA" ]]; then
    echo "ERROR: mirror dev HEAD ($head) != pinned SHA ($SEEDSIGNER_SHA)." >&2
    echo "Either the mirror moved (bump the baseline) or enable fetch-by-SHA." >&2
    exit 1
  fi
fi
git -C "$STAGE/seedsigner" checkout -q FETCH_HEAD

echo "==> staging bundle"
mkdir -p "$STAGE/bundle"
cp -R "$STAGE/seedsigner/src/seedsigner" "$STAGE/bundle/seedsigner"
cp "$STAGE/seedsigner/LICENSE.md" "$STAGE/bundle/SEEDSIGNER-LICENSE.MIT.md"

echo "==> vendoring pure-Python deps: ${PYTHON_DEPS[*]}"
python3 -m pip install -q --no-deps --target "$STAGE/bundle" "${PYTHON_DEPS[@]}"
# pip drops dist-info dirs; keep them (license metadata) but strip caches.
find "$STAGE/bundle" -name '__pycache__' -type d -exec rm -rf {} +

cat > "$STAGE/bundle/PROVENANCE.txt" <<EOF
SeedSigner upstream source, bundled verbatim for the browser sim.
Repo:   https://github.com/SeedSigner/seedsigner
Commit: ${SEEDSIGNER_SHA} (branch dev)
License: MIT (see SEEDSIGNER-LICENSE.MIT.md)
Vendored deps: ${PYTHON_DEPS[*]}
Built by: web/seedsigner-sim/build.sh in mineracks/seedhammer-v1-companion
Note: resources/seedsigner-translations is an upstream git submodule and is
NOT bundled yet (English-only sim). See docs/architecture/BASELINES.md.
EOF

mkdir -p "$OUT"
rm -f "$OUT/seedsigner-bundle.zip"
(cd "$STAGE/bundle" && zip -qr9 - .) > "$OUT/seedsigner-bundle.zip"

size=$(stat -f%z "$OUT/seedsigner-bundle.zip" 2>/dev/null || stat -c%s "$OUT/seedsigner-bundle.zip")
echo "built: $OUT/seedsigner-bundle.zip (${size} bytes)"
echo "serve: python3 ../../tools/serve.py   (COOP/COEP headers required — see web/seedsigner-sim/README.md)"
echo "  →    open http://localhost:38080/seedsigner-sim/"
