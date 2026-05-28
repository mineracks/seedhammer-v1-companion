# seedhammer-v1-companion

**Status:** early development. Phase 1 (composer port) in progress. See
[docs/architecture/](docs/architecture/) for the project plan and design
docs.

A browser-based companion for **SeedHammer v1** hardware (the original
Raspberry-Pi-Zero-based engraver), inspired by Gangleri42's SeedHammer II
fork. Ships three coordinated tools and one optional desktop wrapper.

## What this is

### 1. Plate composer (browser PWA)

Design a stainless steel seed-backup plate from your phone or desktop —
seed words, custom title text, optional logos — then transfer the design
to a real SeedHammer v1 controller via QR code.

The composer renders a pixel-faithful preview using the *same Go code the
Pi controller runs*. What you see in the browser is what the engraver
will physically punch.

The on-the-wire envelope is **SH1E** (a CBOR + CRC32 format documented at
[docs/architecture/sh1e-spec.md](docs/architecture/sh1e-spec.md)). Sized
to fit a 24-word multisig plate in a single QR frame, with BBQr fallback
for larger multi-plate manifests.

### 2. SeedHammer v1 emulator (browser PWA)

Run the real v1 controller firmware in your browser. The same `gui/`,
`input/`, and `engrave/` Go packages that drive the physical device,
compiled to WASM. Use it to:

- Test workflows end-to-end without a physical device
- Take screenshots / record screencasts of v1 flows
- Demo SeedHammer v1 to people without shipping hardware

Keyboard mapping: arrows = joystick, Enter = center/confirm, 1/2/3 = Button1/2/3.

### 3. Bundled SeedSigner emulator (browser PWA)

A faithful in-browser SeedSigner — both the classic 1.3" 240×240 model and
the newer 2.8" SeedSigner+ "jumbo" model. Generates seed-phrase QR codes
that you can hand off to the SeedHammer v1 emulator via a single button
press, end-to-end without leaving the page.

Built by hosting the upstream SeedSigner Python code via Pyodide so that
*the emulator IS the firmware* — when SeedSigner releases new versions
we bump the pinned commit and the sim updates.

### 4. Optional Android wrapper

Kotlin/Gradle shell hosting the composer WASM, for users who want a
plate-design app instead of a PWA. Mirrors the structure of Gangleri42's
SH2 Android companion.

## Hardware targeted

This codebase targets the **original SeedHammer v1** specifically — the
[Pi Zero v1.3 / WaveShare 1.3" 240×240 LCD HAT / MarkingWay engraver]
hardware. **Not** the newer SeedHammer II (RP2040 / TinyGo / SH2E NFC).

For an SH-II companion, use [Gangleri42's fork](https://github.com/Gangleri42/seedhammer)
directly. Most of the inspiration for this project comes from there.

## Status & roadmap

- ☐ **Phase 1** — composer port (Go-to-WASM, SH1E reference encoder, web UI)
- ☐ Phase 2 — v1 emulator (firmware-in-browser, Gangleri42-faithful UI shell)
- ☐ Phase 2.5 — SeedSigner emulator + QR handoff
- ☐ Phase 3 — combined three-pane sim
- ☐ Phase 4 — real-device validation on real v1 hardware
- ☐ Phase 5 (optional) — ColdCard emulator (port from Gangleri42's fork)
- ☐ Phase 6 (optional) — Android wrapper

## Building

(Will be documented as Phase 1 lands — `go build ./...` + a Vite build for
the web shells.)

## License

Released under the [Unlicense](LICENSE) (public domain dedication), matching
upstream SeedHammer's choice. SeedSigner-derived files segregated and
retain their MIT notice.

## Credits + provenance

Heavy lifting by three upstreams. See [CREDITS.md](CREDITS.md) for what
came from where and the pinned baseline commits.
