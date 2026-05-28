# SeedSigner emulator — reuse strategy + jumbo-variant support

This doc covers two scope additions to the SeedSigner sim portion of the
project (Phase 2.5 in the project plan):

1. Reuse of existing SeedSigner artwork and screen flow definitions
2. Support for the "jumbo" SeedSigner+ variant in addition to classic

## TL;DR

- Build the SeedSigner sim by **rendering upstream firmware code directly in
  the browser via Pyodide** — bundle the official `SeedSigner/seedsigner@dev`
  Python with the official screenshot generator's display-mock pattern,
  switch the ST7789 driver for a canvas writer. ~3-4 days for a functional
  build, +1-2 days for chassis art.
- This gets us **pixel-faithful screens for free** because we're literally
  running the upstream Python UI code.
- One firmware, two device profiles — Classic 240×240 portrait and
  SeedSigner+ 320×240 landscape. Same buttons, same flows, just a different
  `(driver, w, h)` tuple. No code-fork needed.
- All reusable artwork (icons, fonts, logo, chassis CAD) is MIT-licensed
  upstream — no permission needed.

## Existing SeedSigner emulators (community survey)

| Project | Lang/Runtime | Browser? | License | Verdict |
|---|---|---|---|---|
| `SeedSigner/seedsigner` `tests/screenshot_generator/` | Python+Pillow | No (headless) | MIT | **Reuse pattern + use as CI test oracle** |
| `enteropositivo/seedsigner-emulator` | Python+tkinter | Desktop, no | **No license** | Reference architecture only, don't copy code |
| `v4ires/seedsigner-emulator` | Fork of above | Desktop, no | No license | Same |
| `SeedSigner/seedsigner-settings-generator` | HTML+JS static | Yes | MIT | Useful as **packaging template** |
| Monero forks (`DiosDelRayo`, `Monero-HackerIndustrial`) | Python+tkinter | No | Various | Pattern reference only |

**Key finding:** No browser-based SeedSigner emulator exists. We're in
greenfield. But the upstream screenshot generator at
`SeedSigner/seedsigner@dev:tests/screenshot_generator/generator.py` is
exactly the architecture we want — it runs the full Python UI with a
mocked ST7789 driver and renders to PNG. Reuse its `MockedDisplay` pattern,
swap PNG output for a `<canvas>` writer via Pyodide.

## Architecture decision: Pyodide vs Go-WASM port

For the SeedHammer composer/emulator the language is Go (compiles to WASM
natively). For SeedSigner the language is Python.

Three options for SeedSigner sim:

| Option | Effort | Faithfulness | Bundle size | Maintenance |
|---|---:|---|---:|---|
| (a) Behavioural-only Go port — just emit the right QR formats | 2d | None — no UI | Tiny | Drift risk on every upstream UI change |
| (b) Hand-rebuild SeedSigner UI in JS/WASM from scratch | 7-10d | Best-effort | Medium | High — every upstream change needs porting |
| (c) **Bundle Pyodide + upstream Python verbatim** | 3-4d | **Identical to real device** | ~5MB gzipped | **Zero drift — bump upstream commit, done** |

**Recommendation: (c)** for the SeedSigner sim, given the user's stated
preference for "great result that matches reality". The 5MB Pyodide bundle
is a one-time download (cached forever in PWA), and we get bit-perfect
fidelity to whatever upstream ships. When upstream releases v0.9.0, we
bump the commit, the sim updates automatically.

This is *the same architecture choice* we made for the SeedHammer composer
(reuse upstream `engrave/`+`font/` packages directly), just expressed in
Python land via Pyodide instead of Go via WASM.

## Reusable assets (all MIT — direct use OK)

### Runtime UI assets
Paths relative to `SeedSigner/seedsigner@dev`:
- `src/seedsigner/resources/icons/` — `arrow-up`, `arrow-down`, `back`,
  `btc_logo`, `btc_logo_30x30`, `btc_logo_bw`, `dire_warning`, `warning`
  (plus `_selected` variants), PNG
- `src/seedsigner/resources/img/` — `btc_logo_60x60.png`, `logo_black_240.png`
- `src/seedsigner/resources/fonts/` — OpenSans, Inconsolata, NotoSans,
  FontAwesome Free Solid, **`seedsigner-icons.otf`** (custom icon font)

### Branding
- `docs/img/logo.svg` — official SeedSigner logo
- `docs/img/Mini_Pill_Main_Photo.jpg`, `Orange_Pill.JPG`,
  `Open_Pill_*.JPG`, `Open_Pill_w_Comfort_Joystick.png` — hardware photos

### Chassis CAD (best for rendering emulator frame)
- `enclosures/open_pill/` — `.f3d` Fusion 360 source + `.stl`
- `enclosures/orange_pill/` — `.f3d` + `.stl` for Upper, Lower, button, joystick
- `enclosures/orange_pill_mini/`, `open_pill_mini[_w_coverplate]/`,
  `look_screws_pill/`, `pushcase/`

**For SeedSigner+ chassis:** no upstream CAD exists. Sold by Go Brrr at
gobrrr.me. Either:
- render a generic landscape-form-factor chassis ourselves
- contact Go Brrr for permission to use their product render
- ship without chassis art initially (just the screen frame), add chassis
  later if Go Brrr provides one or someone in the community models it

## Behavioural spec — also upstream

The most valuable thing we get from Pyodide-bundle-upstream is that
**`src/seedsigner/views/` + `src/seedsigner/gui/screens/` define every
screen flow in the device**. There's no need to re-document or
reverse-engineer flows — they're MIT-licensed code we run directly.

## The screenshot generator as CI oracle

We can wire `pytest tests/screenshot_generator/generator.py` into our
emulator's CI: render every canonical screen with the mocked display,
hash the PNGs, then have our browser emulator render the same screens
via Pyodide and compare. Any pixel divergence = regression.

This is a **stronger** correctness story than any community emulator has.

## Jumbo / SeedSigner+ variant

### Hardware (verified 2026-05-28)

| Field | Classic | SeedSigner+ (jumbo) |
|---|---|---|
| Screen | 1.3" 240×240 | **2.8" 240×320 (rendered as 320×240 landscape)** |
| Driver chip | `st7789` | `st7789` *or* `ili9341` (Go Brrr doesn't publish which; either is supported by firmware) |
| Pi board | Pi Zero v1.3 | **Same — Pi Zero v1.3** |
| Camera | OV5647 (Pi Zero camera) | **Same — OV5647-class** |
| Buttons | 5-way joystick + 3 keys | **Same — identical input pins + semantics** |
| Touchscreen | No | **No** (not in firmware, not in product) |

Released in upstream **v0.8.6 "The Bigger Picture"** (2025-06-30).

Sold by Go Brrr as **"SeedSigner+"** — ~€19 enclosure / ~€130 assembled.
Also available as **"Battery Powered SeedSigner+"** with AAA battery pack.

A 3.5" ILI9486 480×320 variant is **declared but not implemented** in
firmware (`raise Exception("ILI9486 display not implemented yet")`). So a
true jumbo-jumbo exists on the roadmap — we'll plumb for it but won't
expose the profile until upstream lands the driver.

### Firmware reality

**Single codebase, multiple device profiles** — selection happens in
`src/seedsigner/hardware/displays/display_driver.py` via
`DisplayDriverFactory.instantiate_display_driver(display_type, width, height)`.

For the emulator that means **one Python bundle, three (eventually four)
device profiles**:

```yaml
profiles:
  - id: classic
    label: "SeedSigner Classic (1.3\" 240×240)"
    driver: st7789
    width: 240
    height: 240
    orientation: portrait
  - id: plus
    label: "SeedSigner+ (2.8\" 320×240)"
    driver: st7789
    width: 320
    height: 240
    orientation: landscape
  - id: plus-ili9341
    label: "SeedSigner+ ILI9341 panel variant"
    driver: ili9341
    width: 320
    height: 240
    orientation: landscape
  # - id: jumbo-3-5
  #   label: "SeedSigner 3.5\" (planned, not yet supported by firmware)"
  #   driver: ili9486
  #   width: 480
  #   height: 320
  #   orientation: landscape
  #   disabled: true
  #   disabled_reason: "Upstream firmware ili9486 driver not yet implemented"
```

The browser emulator's UI gets a profile selector dropdown. Switching
profiles re-initialises the Pyodide-side `DisplayDriverFactory` with the
new tuple and re-renders. Buttons remain identical across all profiles.

## Items to verify on real hardware

1. Whether the SeedSigner+ panel's actual driver IC is ST7789 or ILI9341.
   Go Brrr doesn't publish it; firmware accepts either. Our emulator
   supports both as separate profiles.
2. Panel orientation handling — firmware reports 240×320 native and renders
   320×240 landscape. Visually verify on a real device that left-edge of
   the rendered image matches the physical "top" of the screen as the user
   holds it.
3. STL licensing for the SeedSigner+ enclosure if we want a chassis render.
   Currently appears Go-Brrr-proprietary.

## Open questions for project plan

1. **License compatibility for Pyodide bundling.** Pyodide itself is MPL-2.0
   licensed. The Python wheels we include depend on each package's license.
   Need to audit: SeedSigner deps + transitive deps for any GPL.
2. **Bundle size budget.** A baseline Pyodide bundle is ~10MB raw / ~3-5MB
   gzipped. Plus SeedSigner code + assets. Need to measure and ensure PWA
   caches sensibly. First load slower; subsequent loads instant.
3. **Camera/QR emulation in Pyodide.** SeedSigner reads QR codes via
   PiCamera2/OpenCV. We need to wire the browser's `<canvas>`-based QR
   handoff to the SeedSigner Python's camera API stub. This is doable but
   isn't trivial. Likely a Python shim that monkey-patches
   `seedsigner.hardware.camera`.

These three are not blockers for Phase 2.5 starting but should be flagged
upfront so we don't discover them mid-build.
