# The Pi fork: SeedHammer v1.3 firmware on SeedSigner hardware, with custom plates and share plates

**Status 2026-10-07:** planned, not built. Everything below the line "what the Pi runs" exists
and is tested; the Pi-side glue and the image build are the remaining work, and they need the
hardware on the desk (a Pi Zero 1.3 + camera + WaveShare LCD, and an engraver to point it at).

## Why a fork, and what it must not do

Clients who own a SeedSigner (Pi Zero 1.3, camera, data port) have exactly the hardware the
SeedHammer v1 firmware runs on. A fork of upstream v1.3.0 gives them an air-gapped engraving
controller for:

1. **Custom plates** — text and line art (labels, logos), designed elsewhere and handed over.
2. **Share plates** — a vault's recovery descriptor split into Shamir shares, one SH-03 plate
   per share, both sides (QR + text), the layout in `plate/share.go`.

**The Pi does no splitting and no crypto.** Shares are made in Any Two Keys (Blockchain
Commons SSKR + Gordian Envelope, `sskrdesc.rs`, tested) and a single share reveals nothing, so
a *share plate design* is safe to carry to the Pi. The Pi is a rasteriser and a serial driver:
it engraves what it is handed, after showing it on the LCD and waiting for hold-to-confirm.
That keeps the fork small and keeps the thing that must be right (the split) in one tested place.

## One engine, three homes

`plate.Build` (this repo) turns an SH1E design into strokes, with the margin rules inside it.
It is compiled to WASM for Any Two Keys and for the browser composer, and to ARMv6 for the Pi.
The same bytes everywhere; the preview on a Mac is the plate the Pi makes.

| Host | Rasteriser | Wire |
|---|---|---|
| Any Two Keys (desktop) | `composer.wasm` → `composerProgram` | `engraver.rs` (serialport) |
| Browser composer | `composer.wasm` | none — emits SH1E for a QR |
| Pi fork | `plate.Build` in Go | upstream `driver/mjolnir` + the jig offset from `platform_rpi.go` |

## Ingestion on the Pi — two doors, both air-gap shaped

**1. QR, by camera (SH1E).** The envelope in `docs/architecture/sh1e-spec.md` (v2: text, SVG
paths, QR blocks; CRC32; design fingerprint). A share plate side is ~630–880 bytes of SH1E,
which is one QR at version ~22–26 — scannable by the Pi camera from a phone or laptop screen,
but at the edge; fall back to multi-part UR fountain frames (upstream's `ur` decoder already
reassembles those for descriptors) when a design is over ~900 bytes. The device shows the
decoded design's preview on the LCD, names the plate type it expects, and the usual
"hold to engrave" flow follows. The companion's emulator already decodes SH1E; the path
"composer → emulator SH1E ingest" is the one still marked unexercised in `BASELINES.md` and is
the first thing to drive end to end.

**2. SD card folder.** `plates/*.sh1e` and `plates/*.svg` on the boot card's FAT partition,
listed in a "From card" menu entry. Upstream already watches the card slot (`initSDCardNotifier`
in `cmd/controller/platform_rpi.go`). An `.svg` is run through the same shape-to-path rewrite
the composer uses (rects, circles, ellipses, lines, polygons; text must be outlined first) and
fitted to the chosen plate. This is the door for line art too big for a camera to read.

## The GUI changes (upstream `gui/gui.go`)

- Main menu gains **"Custom plate"** → "Scan a design" / "From card". Either lands on a
  preview screen (the LCD shows `plate.Preview` rasterised small, plus plate type, stroke
  count, warnings) → existing `EngraveSideA` instructions → engrave.
- Share plates arrive the same way — they are just SH1E designs. The preview screen prints
  the header line it finds ("SHARE 1 OF 3 - ANY 2 RECOVER - <set>") so the user can check the
  share index before mounting a plate.
- Nothing in the seed or descriptor flows changes.

## Build

Upstream builds the Pi image with Nix (`flake.nix`, `nixos-generators`): kernel, firmware,
the Go controller, libcamera. The controller needs cgo for `zbar` (QR decoding), `libcamera`
and `drm`, so it cannot be cross-compiled with a bare `GOOS=linux GOARCH=arm GOARM=6` — the
2026-10-07 attempt confirmed that. The fork therefore builds **inside upstream's flake**: the
fork is upstream v1.3.0 (`2f071c1`) plus our packages (`plate`, `engrave/wire/sh1e`, the
jig-aware driver call) and the GUI changes, with the flake's `go` derivation pointed at the
fork. Needs Nix on the build machine (not installed on the Mac as of 2026-10-07).

## Test plan, in order

1. `plate` tests (done, `go test ./plate`), composer WASM exports from Node (done).
2. Emulator: composer → SH1E QR → `cmd/emulator` decodes and previews (not yet exercised).
3. Hardware, pen up: SH1E on the LCD → trace with `-dry` through the fork, plate framed.
4. Hardware, pen down: a share plate side A on an SH-03; **scan it with a SeedSigner camera**
   and with a phone. This decides module size (0.9 mm) and EC level (L) for real.
5. Side B; type it back through Any Two Keys' Restore (`share_rewrap`) to prove the text path.
6. The SD door with a logo SVG.

## Open questions for Piers

- The fork's name and where it lives: this repo already carries the lifted packages; the
  cleanest is a `mineracks/seedhammer-pi` fork of upstream that vendors `plate/` and `sh1e/`
  from here, so this repo stays the engine and the Pi repo stays a thin fork.
- Whether the SD door is wanted at all for client devices (a card slot is also an exfiltration
  path on an air-gapped device). The QR door alone covers share plates and labels.
