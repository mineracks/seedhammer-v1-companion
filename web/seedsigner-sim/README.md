# SeedSigner sim (Phase 2.5)

Upstream SeedSigner firmware running verbatim in the browser via Pyodide —
*the emulator IS the firmware*. Design rationale and the option matrix live
in [docs/architecture/seedsigner-reuse.md](../../docs/architecture/seedsigner-reuse.md);
the pinned upstream SHA lives in
[docs/architecture/BASELINES.md](../../docs/architecture/BASELINES.md).

## Build + run

```sh
./build.sh                    # vendor/seedsigner-bundle.zip from the pinned SHAs
./vendor-pyodide.sh           # vendor/pyodide/ + vendor/jsqr.js (offline runtime)
python3 ../../tools/serve.py  # COOP/COEP dev server on :38080
# → http://localhost:38080/seedsigner-sim/
```

`vendor/seedsigner-bundle.zip` is upstream `src/seedsigner/` (MIT, notice
retained in the zip) plus the pure-Python deps from upstream's
requirements.txt (`embit`, `qrcode`, `urtypes`), plus the translations repo
(21 locales, `.po` compiled to `.mo` with msgfmt at build time) and its
script fonts (Noto CJK/AR/TH). Don't edit bundle contents — bump the SHAs
in BASELINES.md + build.sh and rebuild instead.

`vendor-pyodide.sh` pins the Pyodide runtime + pillow/numpy/hashlib wheels
(`hashlib` is Pyodide's unvendored OpenSSL module — without it
`hashlib.pbkdf2_hmac` is missing and seed derivation crashes) + jsQR.
worker.js prefers the vendored copies and falls back to the jsdelivr CDN.

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
- display driver factory → `CanvasDisplay` (frames → `postMessage` → canvas);
  device profile (Classic / SeedSigner+ st7789 / ili9341) selected on the
  page, written to `settings.json` pre-boot
- `time.sleep` → `Atomics.wait` (real blocking sleep; the firmware's 10ms
  input poll would otherwise peg a core)
- `Camera` → fake fed RGB frames through a camera SAB the main thread pumps
  from getUserMedia (or a dropped image file); the worker can't receive
  postMessages while Python runs, hence the SAB
- `pyzbar.decode` → jsQR on the JS side; `.data` comes from jsQR's
  binaryData so both text SeedQRs and binary CompactSeedQRs work
- `BackgroundImportThread` runs inline (Controller.storage deadlocks without
  it); all other threads are skipped — Pyodide can't start threads. The
  scan screen's LivePreviewThread is one of those, so the page overlays the
  real `<video>` on the LCD while the camera is open
- `picamera`/`microsd` → MagicMock

## QR handoff to the SeedHammer v1 emulator

"Hand off QR → SeedHammer" jsQR-decodes whatever QR is on the LCD,
stores `{payload, frame}` under `localStorage["sh1-handoff"]`, and opens
`/emulator/`. The emulator injects it via `emulatorInjectQR` (Go side:
`CameraFrame` serves the preview frame, `ScanQR` returns the payload
one-shot), so the firmware's own ur/nonstandard/seedqr decoder chain
consumes it. Verified e2e: scan test SeedQR into SeedSigner → Backup seed →
Export as SeedQR → handoff → SeedHammer "Confirm Seed" shows the words.

## Why a special dev server

SharedArrayBuffer requires cross-origin isolation (COOP/COEP headers).
`python3 -m http.server` can't send them — the page will show
"Not cross-origin isolated" and stop. `tools/serve.py` is the same thing
plus the three headers. For static hosting later (e.g. GitHub Pages-style),
use a `coi-serviceworker` shim or hosting with header support.

## Known gaps

- **Toasts**: toast manager threads are skipped (no threads in Pyodide) —
  SD-card/"remove card" toasts never appear. Cosmetic.
- **Scan preview text**: the firmware draws scan-progress % via
  LivePreviewThread (skipped); the `<video>` overlay is at 0.85 opacity so
  LCD text ghosts through, but animated-QR progress is less visible.
- **PWA/offline install**: everything is served same-origin now, but
  there's no service worker yet, and static hosts need COOP/COEP (or a
  coi-serviceworker shim).
- **ili9486 480×320**: declared upstream but not implemented in their
  driver; profile intentionally not exposed.
- **libraqm**: absent in Pyodide's Pillow — complex-script shaping (Arabic,
  Thai, Devanagari) may differ slightly from device renders.
