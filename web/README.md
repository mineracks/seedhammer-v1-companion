# web/

Static-site assets for each browser target.

| Path | What it ships |
|---|---|
| `composer/` | The plate composer PWA (uses `cmd/composer/` WASM) |
| `emulator/` | The v1 emulator PWA (uses `cmd/emulator/` WASM) |
| `combined/` | The three-pane combined sim (uses all three) |
| `seedsigner-sim/` | The Pyodide-hosted SeedSigner emulator |
| `shared/` | Common CSS/JS/assets used by multiple shells |

Each subdirectory has its own `index.html`, `app.js`, `app.css`,
`manifest.webmanifest`, `sw.js`, modelled on Gangleri42's PWA shells.

Build pipeline (TBD — likely Vite or a small `make` rule that wires
`go build -o app.wasm ./cmd/X` and copies static files to `dist/`).

Status: skeleton only; shells lifted in Phase 1.
