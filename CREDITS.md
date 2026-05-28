# Credits

This project stands on the shoulders of three communities. None of these
upstreams legally require attribution (the SeedHammer projects are released
under the Unlicense; SeedSigner is MIT). Attribution is kept here as
courtesy and to make code provenance traceable.

## Upstream SeedHammer (Pi-Zero / v1 firmware)

- **Repo:** https://github.com/seedhammer/seedhammer
- **Baseline:** v1.3.0 (commit `2f071c1d8f23eb7fd39b15fc0acb8874113f801e`)
- **License:** Unlicense (public domain dedication)
- **Lifted from this codebase:** plate-area constants (`backup/`), engrave-stroke
  geometry (`engrave/`), MarkingWay USB-serial driver (`driver/mjolnir/`),
  WaveShare HAT button mapping (`driver/wshat/`), pre-rasterised OpenType
  fonts (`font/comfortaa`, `font/poppins`, `font/constant`), GUI screen flows
  (`gui/`), camera + LCD drivers (`driver/libcamera/`, `driver/drm/`),
  curve math (`bezier/`, `bspline/`), image helpers (`image/`).

## Gangleri42's SeedHammer fork (SH-II features that inspired this port)

- **Repo:** https://github.com/Gangleri42/seedhammer
- **Baseline:** branch `seedhammer-features` (commit `0a3c63efb125d17d8ec86ce739ecd058c8747cfe`)
- **License:** Unlicense (public domain dedication)
- **Lifted from this codebase:** composer PWA shell (`cmd/webnfc/`), firmware-in-browser
  emulator pattern (`cmd/wasmemu/`), combined composer+emulator harness
  (`cmd/webnfc-sim/`), Android wrapper structure (`cmd/seedhammer-android/`).
  The engrave-payload encoder (`engrave/wire/`) is NOT lifted as-is — it is
  SH-II-specific (the SH2E NFC envelope); v1 has a completely different live
  USB-serial protocol. We replace it with a v1-shaped SH1E envelope (see
  `docs/architecture/sh1e-spec.md`).

## SeedSigner (companion emulator)

- **Repo:** https://github.com/SeedSigner/seedsigner
- **Baseline:** TBD — bundled via Pyodide at a pinned commit; see
  `web/seedsigner-sim/UPSTREAM.md` once that is wired up.
- **License:** MIT
- **Used in this project:** runtime UI assets (`src/seedsigner/resources/`),
  custom icon font (`seedsigner-icons.otf`), screen-flow definitions
  (`src/seedsigner/views/`, `src/seedsigner/gui/screens/`), screenshot
  generator (`tests/screenshot_generator/`) used as a CI pixel-diff oracle,
  3D-printable enclosure CAD source (`enclosures/`) for the device chassis
  render.
- **License compliance:** SeedSigner-derived files segregated under
  `web/seedsigner-sim/upstream/` and `cmd/seedsigner-sim/upstream/` with
  the MIT notice preserved in `LICENSE.seedsigner`.

## Other dependencies

- **`github.com/fxamacker/cbor/v2`** — MIT — used for SH1E payload
  encoding (canonical CBOR).
- **`github.com/tarm/serial`** — MIT — USB-serial driver, brought in via
  upstream SeedHammer's `driver/mjolnir/`.
- **`periph.io/x/conn/v3`** + **`periph.io/x/host/v3`** — Apache 2.0 — GPIO
  library, brought in via upstream SeedHammer's `driver/wshat/`.
- **Pyodide** — MPL-2.0 — Python-in-WASM runtime used to host the
  SeedSigner emulator in-browser. See `web/seedsigner-sim/pyodide/`.

## Contributing

If you contribute and want your name listed here, just say so in your PR.
We will not list you without consent.
