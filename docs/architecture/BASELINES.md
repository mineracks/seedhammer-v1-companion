# Project baselines (frozen 2026-05-28)

These are the immutable starting points for the SeedHammer v1 companion port.
Always rebase against these exact SHAs unless deliberately bumping the
baseline — and if bumping, append a new dated section below, don't
overwrite this one.

## Upstream v1 (seedhammer/seedhammer @ v1.3.0)

| | |
|---|---|
| Tag | `v1.3.0` |
| Tag object SHA | `70ed718c797663beeb65d323340e3853fca7920f` |
| **Commit SHA** | **`2f071c1d8f23eb7fd39b15fc0acb8874113f801e`** |
| Repo | https://github.com/seedhammer/seedhammer |
| Why this tag | **Verified 2026-05-28**: v1.3.0 is the latest tag still targeting v1 hardware (Pi Zero / WaveShare 1.3" LCD HAT / mjolnir USB-serial engraver). All five v1.x tags (v1.0.0 through v1.3.0) are v1-hardware; v1.4.x+ are SeedHammer II (RP2040 / TinyGo / SH2E wire protocol). See "Baseline bump 2026-05-28" section below for the verification evidence. |

### Other available v1.x tags

All verified v1-hardware on 2026-05-28; later v1.x tags supersede earlier
ones with bug fixes and a directory-layout refactor (see bump note below).

```
v1.0.0  → 6f9aa7ac017db3978703e596c77e7919c7fdaa8d   (original baseline; superseded)
v1.1.1  → 5b0265deae09117e9949ec8a1b76cc0ccc7bded0   (v1-hardware, pre-refactor layout)
v1.2.0  → 595a8c04574a6694e791be269710c3e81a7bab0a   (v1-hardware, post-refactor layout)
v1.2.1  → f5c41e3a1b4ba815dfdee726d11b702d7cc4bc22   (v1-hardware, post-refactor layout)
v1.3.0  → 2f071c1d8f23eb7fd39b15fc0acb8874113f801e   ← chosen baseline
```

### Baseline bump 2026-05-28: v1.0.0 → v1.3.0

Verified by direct GitHub-tree + raw-content inspection of all five v1.x
tags. **All are v1 hardware.** Evidence:

- `go.mod` at v1.3.0 still imports `github.com/tarm/serial` (v1 USB-serial
  engraver lib) and `periph.io/x/conn/v3` + `periph.io/x/host/v3`
  (BCM283x GPIO lib, used by `bcm283x.GPIO6/19/5/26/13/21/20/16` for the
  WaveShare HAT joystick + 3 keys — same pin map as v1.0.0).
- `flake.nix` at v1.3.0 references only `raspberrypi` (kernel, firmware,
  camera modules 1/2/3, Pi Zero 2). **No TinyGo, no RP2040, no pico**.
- README at v1.3.0 explicitly: *"It runs on the same hardware as the
  SeedSigner: Raspberry Pi Zero or Zero W, a WaveShare 1.3 inch 240x240
  LCD hat and a Pi Zero compatible camera with a OV5647 sensor."*
- `driver/mjolnir/driver.go` at v1.3.0: package comment *"driver for the
  MarkingWay engraving machine"*, still uses `tarm/serial`.
- No `picobin/`, `uf2/`, `cmd/picosign`, `cmd/biptool`, `engrave/wire/`,
  or `nfc/` at any v1.x tag — all are SeedHammer II markers.

**Layout-refactor caveat (lift-path updates needed):** v1.2.0 reorganised
the tree without changing hardware. The "lift from upstream" paths below
have been updated to the v1.3.0 layout. v1.0.0 → v1.3.0 path mapping:

| v1.0.0 path | v1.3.0 path |
|---|---|
| `input/input.go` | `driver/wshat/wshat.go` |
| `mjolnir/driver.go`, `mjolnir/sim.go` | `driver/mjolnir/driver.go`, `driver/mjolnir/sim.go` |
| `lcd/lcd_linux.go` | `driver/drm/drm_linux.go` |
| `camera/camera_linux.{cpp,go,h}` | `driver/libcamera/camera_linux.{cpp,go,h}` |
| `rgb16/`, `ninepatch/` | `image/rgb565/`, `image/ninepatch/` |
| `cmd/controller/platform_linux.go` | `cmd/controller/platform_rpi.go` |

Top-level diff: v1.0.0 has 229 tree entries, v1.3.0 has 285. Net adds:
`driver/{drm,libcamera,mjolnir,wshat}`, `image/{alpha4,image.go,ninepatch,paletted,rgb565}`,
`font/{bitmap,vector}`, `bip39/{gen.go,wordlist.go}`. Removed: `affine/`,
`camera/`, `input/`, `lcd/`, `mjolnir/` (top-level), `ninepatch/`, `rgb16/`
(all subsumed by the refactor). go.mod adds `github.com/kortschak/qr`
(replaces `skip2/go-qrcode`) and `decred/dcrec/secp256k1/v4` as direct dep.

The button-pin map (`GPIO 6/19/5/26/13/21/20/16` for Up/Down/Left/Right/
Center/Button1/Button2/Button3) is **identical byte-for-byte** between
v1.0.0 and v1.3.0 — just moved file. Likewise the mjolnir wire protocol
constants (`StrokeWidth: 38, Millimeter: 126`) are unchanged.

**Recommendation taken**: bump to v1.3.0. Rationale: same hardware, three
extra release-cycles of upstream bug-fixes, no risk of v2 transition
code (none exists at any v1.x tag), and the post-refactor layout is what
any future upstream cherry-picks will use.

## Gangleri42 fork (Gangleri42/seedhammer @ seedhammer-features)

| | |
|---|---|
| Branch | `seedhammer-features` |
| HEAD SHA | `0a3c63efb125d17d8ec86ce739ecd058c8747cfe` |
| HEAD date | 2026-05-22 |
| Repo | https://github.com/Gangleri42/seedhammer |

### Fork-point note

The fork's history is a **rewrite**, not a fork-from-tip. GitHub's `compare`
API returns 404 / null between upstream `main` and this branch because they
share no common ancestor near tip. Treating it as a separate codebase to
cherry-pick from, not a branch to merge.

Lifting strategy:
1. Identify the files / packages we want from `seedhammer-features` at the
   pinned HEAD (see [v1-engrave-spec.md](v1-engrave-spec.md) and
   [v1-buttons-and-ui.md](v1-buttons-and-ui.md))
2. Copy them as-is into the new repo with attribution + license preserved
3. Rebind the v1-incompatible bits (geometry constants, wire format,
   button mapping) per the prep docs

## Key files we plan to lift from Gangleri42 fork

(All paths relative to fork root at the pinned HEAD)

- `cmd/webnfc/` — composer PWA shell + Go-to-WASM entry
- `cmd/wasmemu/` — firmware-in-browser emulator (UI shell will be reused;
  the WASM contents must be rebuilt against v1's `gui/` package)
- `cmd/webnfc-sim/` — combined composer + emulator
- `cmd/coldcard-sim/`, `cmd/coldcard-wasm/` — optional ColdCard emu
- `engrave/wire/` — **NOT** lifted directly; this is SH-II's SH2E format.
  v1 has a completely different live-USB-serial protocol (see
  [v1-engrave-spec.md](v1-engrave-spec.md))
- `bezier/`, `bspline/`, `font/sh/`, `backup/`, `internal/golden/` — lift
  as-is; these are geometry/font math, hardware-agnostic
- `gui/gridedit.go`, `gui/precompiled.go` — composer GUI helpers, lift
  as-is if they remain decoupled from the v2 `gui/` package state

## Key files we plan to lift from upstream v1.3.0

(All paths relative to upstream root at the pinned commit
`2f071c1d8f23eb7fd39b15fc0acb8874113f801e`. If you cross-reference older
prep docs that still cite v1.0.0 paths, see the path-mapping table in the
"Baseline bump 2026-05-28" section above.)

- `driver/wshat/wshat.go` — v1 button + GPIO mapping (was `input/input.go`
  at v1.0.0; see [v1-buttons-and-ui.md](v1-buttons-and-ui.md))
- `gui/` — the v1 controller UI (Linux/Pi-side, full Go, not TinyGo)
- `driver/mjolnir/driver.go`, `driver/mjolnir/sim.go` — v1 engraver driver
  + sim (was `mjolnir/...` at v1.0.0; see [v1-engrave-spec.md](v1-engrave-spec.md))
- `driver/drm/drm_linux.go` — v1 LCD output (was `lcd/lcd_linux.go` at v1.0.0)
- `driver/libcamera/` — v1 camera (was `camera/` at v1.0.0)
- `font/comfortaa`, `font/poppins`, `font/constant` — v1 fonts
  (pre-rasterised OpenType); v1.3.0 also adds `font/bitmap` + `font/vector`
- `image/rgb565/`, `image/ninepatch/` — moved from top-level `rgb16/` +
  `ninepatch/` in the v1.2.0 refactor
- `backup/backup.go` — v1 plate-size constants (SmallPlate / SquarePlate / LargePlate)

## License audit (verified 2026-05-28)

Both repos use the **Unlicense** (public-domain dedication), not MIT as
originally assumed. Confirmed by fetching `LICENSE` from each pinned ref:

- Upstream `seedhammer/seedhammer` @ `v1.3.0` — Unlicense
  - https://raw.githubusercontent.com/seedhammer/seedhammer/v1.3.0/LICENSE
  - File header: *"This is free and unencumbered software released into
    the public domain."* with reference to https://unlicense.org
- `Gangleri42/seedhammer` @ `seedhammer-features` — Unlicense
  - https://raw.githubusercontent.com/Gangleri42/seedhammer/seedhammer-features/LICENSE
  - Byte-identical to upstream's LICENSE file (Gangleri42 inherited it).

Implications for the new repo:

- **No attribution legally required.** Both upstreams have explicitly
  dedicated their work to the public domain.
- We will **still** preserve `CREDITS.md` and inline file-level attribution
  blocks anyway, as a matter of good practice and community courtesy. The
  goal of this port is to *amplify* the original projects, not to obscure
  their origin.
- The new repo will adopt **Unlicense** as well, for full pipeline
  compatibility and zero friction for downstream forks.

For the SeedSigner assets we plan to bundle (see
[seedsigner-reuse.md](./seedsigner-reuse.md)), the upstream
`SeedSigner/seedsigner` repo is MIT-licensed. That requires us to keep the
MIT attribution alongside any SeedSigner assets we ship. Mixed-license
shipping is fine — we'll segregate SeedSigner-derived files under a
clearly-labelled subdirectory with the MIT notice retained.

## How to use this file

- Pin every PR against these SHAs (call them out in the PR body).
- If we deliberately bump:
  - Don't edit the section above. Add a new section below dated and
    cross-link old → new.
  - Re-run the prep checks (engrave spec + button layout) against the
    new SHAs in case anything changed.
- These SHAs are also the input to a deterministic `flake.nix` or
  `vendor.txt` pin if we adopt one.
