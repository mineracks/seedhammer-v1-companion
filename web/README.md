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

Build: each shell has its own `build.sh` (Go-to-WASM for composer/emulator;
vendor-bundle zip for seedsigner-sim).

Serve with `python3 ../tools/serve.py` (NOT plain `http.server`) — the
SeedSigner sim needs COOP/COEP headers for SharedArrayBuffer; they're
harmless for the Go-WASM shells.

Status: composer + emulator + seedsigner-sim live; `combined/` not started.
