# SeedSigner sim (Phase 2.5)

Upstream SeedSigner firmware running verbatim in the browser via Pyodide —
*the emulator IS the firmware*. Design rationale and the option matrix live
in [docs/architecture/seedsigner-reuse.md](../../docs/architecture/seedsigner-reuse.md);
the pinned upstream SHA lives in
[docs/architecture/BASELINES.md](../../docs/architecture/BASELINES.md).

## Build + run

```sh
./build.sh                    # produces vendor/seedsigner-bundle.zip from the pinned SHA
python3 ../../tools/serve.py  # COOP/COEP dev server on :38080
# → http://localhost:38080/seedsigner-sim/
```

`vendor/seedsigner-bundle.zip` is upstream `src/seedsigner/` (MIT, notice
retained in the zip) plus the pure-Python deps from upstream's
requirements.txt (`embit`, `qrcode`, `urtypes`), pip-vendored at pinned
versions. Don't edit bundle contents — bump the SHA in BASELINES.md +
build.sh and rebuild instead.

## Architecture

```
index.html / app.js          main thread: canvas, keyboard, on-screen buttons
        │  SharedArrayBuffer (button bitmask) + frame postMessages
worker.js                    Pyodide; registers the `shsim` JS bridge
        │
boot.py                      hardware shims, then Controller.get_instance().start()
        │
/app/seedsigner/...          upstream firmware, unmodified
```

`boot.py` is the only divergence point from upstream:

- `RPi.GPIO` → fake module reading the SharedArrayBuffer bitmask
- display driver factory → `CanvasDisplay` (frames → `postMessage` → canvas)
- `time.sleep` → `Atomics.wait` (real blocking sleep; the firmware's 10ms
  input poll would otherwise peg a core)
- `BackgroundImportThread` runs inline (Controller.storage deadlocks without
  it); all other threads are skipped — Pyodide can't start threads, so
  toasts/microsd-watcher are cosmetic no-ops for now
- `picamera`/`pyzbar`/`microsd` → MagicMock (same set upstream's screenshot
  generator mocks)

## Why a special dev server

SharedArrayBuffer requires cross-origin isolation (COOP/COEP headers).
`python3 -m http.server` can't send them — the page will show
"Not cross-origin isolated" and stop. `tools/serve.py` is the same thing
plus the three headers. For static hosting later (e.g. GitHub Pages-style),
use a `coi-serviceworker` shim or hosting with header support.

## Known gaps (tracked for later sub-phases)

- **Camera/QR scan**: mocked. Plan: browser camera + JS QR decode feeding a
  Python camera shim (seedsigner-reuse.md, open question 3).
- **QR handoff to the v1 emulator**: the Phase 2.5 headline feature; needs
  the export-QR screens working first (they use `qrcode`, already bundled).
- **Toasts/screensaver threads**: skipped (no threads in Pyodide).
- **Translations**: upstream keeps them in a git submodule we don't mirror
  yet → English-only.
- **Pyodide from CDN**: ~10MB first load, cached after. Vendor it locally
  before any offline/PWA claim.
- **Device profiles**: Classic 240×240 only so far; the SeedSigner+ 320×240
  profiles plug in via the same `CanvasDisplay` (see profile table in
  seedsigner-reuse.md).
