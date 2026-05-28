# SeedHammer v1 engrave wire format

Reverse-engineered by reading the upstream Go controller source at the v1.0.0
tag. This is the protocol the Raspberry Pi Zero speaks over USB-serial to the
MarkingWay engraving machine.

## Source

- Upstream repo: `github.com/seedhammer/seedhammer`
- Tag: **v1.0.0** (commit `6f9aa7a`, released 29 June 2024)
- Why v1.0.0 and not v1.4.x: from v1.4.0 onward the upstream repo became the
  SeedHammer II firmware. v1.0.0 is the last tag that is unambiguously the
  original v1 hardware (Pi Zero + MarkingWay engraver + the same
  configuration as SeedSigner; see `cmd/controller/main.go:1-3`).

Key files:

| Path | LOC | Role |
|---|---|---|
| `mjolnir/driver.go` | 1–418 | The complete engraver wire driver. Opens the serial port, runs the init/program/finish state machine, defines `Program.Move` / `Program.Line`. |
| `mjolnir/sim.go` | 1–204 | Reference simulator. Independently confirms every opcode and the response sequence — treat it as the canonical decoder. |
| `engrave/engrave.go` | 1–1217 | `Program` interface (`Move(image.Point)` / `Line(image.Point)`); rasterises text, QR (constant-time and standard), shapes into Move/Line. |
| `font/font.go` | 1–78 | Glyph format: pre-decoded OpenType segments (MoveTo/LineTo/QuadTo/CubeTo) with float32 coords, ASCII-only index. |
| `backup/backup.go` | 1–80 | Plate size table and outer/inner margins (mm). |
| `gui/gui.go` | 1086–1130 | Where `mjolnir.Engrave(...)` is actually called: per-side `Program{DryRun}` constructed, glyph commands streamed through `Engrave(...)`. |

The driver package is named **mjolnir** (Thor's hammer) — that's the v1
codename for the engraver subsystem; do not look for a package called
`engraver` or `driver`.

## Transport

USB-serial via [`github.com/tarm/serial`](https://github.com/tarm/serial).
Connection params hard-coded in `mjolnir/driver.go:44-83`:

| Setting | Value |
|---|---|
| Baud | **115200** |
| Word length | 8 |
| Stop bits | 1 |
| Parity | none |
| Flow control | none (handshake=0, replace=0) |
| Xon limit | 2048 |
| Xoff limit | 512 |

Device path:

- Linux (Pi Zero): `/dev/ttyUSB0`, falls back to `/dev/ttyUSB1`.
- Windows (dev/test): `COM3`.

The MarkingWay engraver therefore presents to the Pi as a USB-serial CDC
device. There is no GPIO/SPI involvement at the controller-to-engraver layer
(the Pi's GPIO is used only for the LCD/buttons inside the SeedSigner-style
case, not for engraver I/O).

Handshaking is **command/response at the application layer**, not at the
serial layer. Every controller-initiated command produces a 1-or-more-byte
echo or status from the engraver (see opcode table). There is no XON/XOFF or
RTS/CTS; the controller-side buffer is `bufio.NewWriterSize(dev,
progBatchSize*cmdSize) = 80*10 = 800 bytes`.

## Wire format

**Binary, fixed-width, command-tagged.** No framing, no checksum, no
length prefix, no ACK byte. The receiver is a simple byte-driven state
machine that knows how many bytes follow each opcode.

### Two distinct phases

1. **Control phase.** Variable-length commands with immediate echo. Used
   for init, set speed, set delays, move-to-origin, query position, and to
   enter program mode.
2. **Program phase.** Fixed 10-byte commands ("draw commands"), streamed
   in **batches of 80 commands (800 bytes)** without per-command ACK. The
   engraver sends a 1-byte status byte after each batch (`bufferProgramStatus
   0x60`) requesting the next batch, plus one `programStepStatus 0x6f` per
   completed step and a final `programCompleteStatus 0x6a`. From
   `mjolnir/driver.go:108-109` and `:240-305`.

### Draw command layout (program phase, 10 bytes)

From `mjolnir/driver.go:370-380` (`mkcoords`) and `:394-418`
(`Move` / `Line`):

```
byte 0      : opcode  (0x80 = MoveTo / pen up, 0x00 = LineTo / pen down)
bytes 1..3  : X coordinate, 24-bit little-endian unsigned
bytes 4..6  : Y coordinate, 24-bit little-endian unsigned
bytes 7..9  : Z coordinate, 24-bit little-endian unsigned (always 0 in v1)
```

Concrete example: move pen to (10 mm, 5 mm). With `Millimeter = 1/0.00796 ≈
125.628`, 10 mm = 1256 ≈ `0x0004E8`, 5 mm = 628 ≈ `0x000274`. On the wire:

```
80  E8 04 00   74 02 00   00 00 00
```

LineTo to the same point: `00 E8 04 00 74 02 00 00 00 00`.

Coordinate range: `0` to `0xFFFFFF` (24-bit unsigned). Negative coords
panic (`driver.go:372-374`). Z is reserved — the controller always sends
zero and the simulator never reads it (`sim.go:80-84` only parses X and Y).

Padding: if a batch isn't full, the controller pads with **`0xFF`** bytes
(NOP, `nopCmd`). The engraver treats `0xFF` as a no-op in program mode
(`sim.go:179-180`). The padding is critical: omit it and the engraver
won't emit the completion status (`driver.go:243-245` — "Otherwise, the
engraver won't send a completed status").

### Opcode table

All values from `mjolnir/driver.go:85-106`. Echo/response columns from
`sim.go:86-119` and the `expect(...)` calls in `driver.go:185-220`.

| Opcode | Name | Args (bytes after opcode) | Reply | Phase | Notes |
|---|---|---|---|---|---|
| `0x00` | `initCmd` | none in control phase; in program phase the leading `0x00` is `lineCmd` and is followed by 9 coord bytes | After init: a status byte loop (see status table) | Control / Program | Same byte serves two purposes depending on phase. `sim.go:149-157` makes this explicit. |
| `0x16` | (un-named "query position") | none | Echoes `0x16`, then 9 coord bytes (X/Y/Z each 24-bit LE) | Control | Defined in `driver.go:215-220` but the function is bound to `_` — present in the protocol, not used by v1.0.0 firmware. |
| `0x21` | `moveToOriginCmd` | 1 byte: `0x50` (`moveToOriginCmdExtra`) | Echoes `0x21 0x00` (`moveToOriginCmdResponse`) | Control | "Reset origin to current physical position" (`driver.go:322-327`). Called twice in `Engrave`: before and after the needle-warmup pass. |
| `0x30` | `setSpeedCmd` | 6 bytes: print(LE16), move(LE16), `xxx`(LE16). Speed range `[1000, 30]` where **lower = faster** | Echoes `0x30` | Control | `driver.go:226-229`. v1 always passes `xxx = 0xE6` (230) — purpose undocumented, possibly a Z-axis or acceleration parameter. Called three times in `Engrave`: 300/300 for warmup, then user move/print speeds, then 300/300 for the post-engrave move-to-end. |
| `0x31` | `setDelaysCmd` | 2 bytes: penDown delay, penUp delay (0–255) | Echoes `0x31` | Control | `driver.go:232-235`. v1 hard-codes `setDelays(0x14, 0x14)` = (20, 20). |
| `0x60` | `initProgramCmd` | 2 bytes: nbatches (LE16). Each batch = 80 × 10-byte commands = 800 bytes | None directly — engraver then drives the program-phase loop via status bytes | Control → Program | `driver.go:250` and `:171-174` in the sim. nbatches must be ≤ 0xFFFF (program-too-large guard at `driver.go:246-249`). |
| `0x80` | `moveCmd` | 9 coord bytes (X/Y/Z LE24) | None (batched) | Program | Pen-up move. |
| `0x00` | `lineCmd` | 9 coord bytes (X/Y/Z LE24) | None (batched) | Program | Pen-down draw / "hammer along path". Note this is the same byte as `initCmd`; phase-disambiguated. |
| `0xAF` | `cancelCmd` | none | Engraver transitions through `cancellingStatus` to `cancelledStatus` | Any | Sent on the quit channel (`driver.go:147`). |
| `0xFF` | `nopCmd` | 9 ignored bytes (treated as filler) | None (batched) | Program | Pad byte for incomplete batches. |

### Status byte table (engraver → controller)

From `driver.go:99-106` and `sim.go:108-119`. These are always **single
bytes** read by the controller; there's no length prefix.

| Status | Name | When sent | Controller reaction |
|---|---|---|---|
| `0x00` | `initializedStatus` | After `initCmd` succeeds | Exit init loop. |
| `0x60` | `bufferProgramStatus` | When the engraver's command buffer is ready for the next 80-command batch | Send 80 × 10-byte commands (with 0xFF padding if needed). |
| `0x62` | `cancellingStatus` | After receiving `cancelCmd`, before stopping | Wait. |
| `0x65` | `cancelledStatus` | Cancel complete | Set `ErrCancelled`. During init the controller responds by re-sending `initCmd` (`driver.go:200-206`). |
| `0x6A` | `programCompleteStatus` | Final batch consumed | Break out of program loop. |
| `0x6F` | `programStepStatus` | After each draw command executes | Used to drive the progress bar (`driver.go:282-295`); the controller throttles updates to every 10th step or the last step. |

Note `0x00` is **both** the init-success status and the `lineCmd` opcode.
The driver disambiguates by remembering which phase it's in
(`stateExecuting` vs everything else, `sim.go:148-157`).

### Connection lifecycle (one full engrave job)

The order is documented by `mjolnir.Engrave(...)` in `driver.go:111-366`:

1. `cancel()` → `wr(initCmd)`, loop on status until `initializedStatus`.
2. `setSpeeds(300, 300, 0xE6)` — warmup speeds.
3. `setDelays(0x14, 0x14)`.
4. `origin()` — reset the origin (engraver assumes current physical
   position is (0,0)). Required because the engraver does not retain
   absolute position across power cycles (`driver.go:322-327`).
5. **Needle-warmup pass**: a tiny 3-step program that walks
   `(0,0) → line → move → line → move → line → move` out to (10 mm, 10 mm)
   in 3 segments, to "exercise the needle" — "some machine needles are
   stuck for the first few engravings" (`driver.go:325-345`).
6. `origin()` again (resets origin to back to (0,0) physically).
7. `setSpeeds(printSpeed, moveSpeed, 0xE6)` — user speeds.
8. `runProgram(prog, progress)` — the actual engrave: stream nbatches × 80
   draw commands, padded with 0xFF.
9. `setSpeeds(300, 300, 0xE6)` again.
10. `moveTo(prog.End)` — park the head at the user-supplied end point.

The `MoveSpeed` / `PrintSpeed` fields on `Program` are normalised in
`[0,1]` where 0 = lowest, 1 = highest, and mapped to engraver units by
`speed = printSpeed*30 + (1-printSpeed)*1000` (`driver.go:347-358`). So
on the wire the engraver wants **smaller numbers = faster** (30 fast, 1000
slow). Defaults: `defaultMoveSpeed = 0.5`, `defaultPrintSpeed = 0.1`
(`driver.go:40-42`).

## Plate geometry

### Coordinate system

- **Units on the wire: machine steps.** One machine step = `0.00796 mm`
  (`mjolnir.Step` in `driver.go:30-35`). The inverse `Millimeter = 1/Step
  ≈ 125.628 steps/mm` is the scale passed into `backup.Engrave`
  (`gui/gui.go:1012`).
- **Origin: physical needle position at the moment `moveToOriginCmd`
  fires.** The engraver has no absolute encoder; the user is expected to
  jog the head to the plate's bottom-left fiducial before engraving (this
  is the "EngraveSideA" GUI step in `gui.go:1029-1036`).
- **Axes orientation**: X right, Y up in the controller's image-space.
  Coordinates are 24-bit unsigned on the wire → effectively only the
  positive quadrant is addressable, which matches a plate fixed at the
  origin.
- **StrokeWidth**: `0.3 mm` (`mjolnir.StrokeWidth`,
  `driver.go:30-31`). This is the punch-impression width assumed for
  hatching and font stroking — not a wire parameter, but it propagates
  into the rasterisation done in `engrave.go` and so determines how many
  Move/Line commands a glyph produces.

### Plate sizes (`backup/backup.go:23-57`)

Three SKUs, all expressed in **millimetres of usable engrave area** (the
metal plate is larger; these are the rectangles the engraver paints in).

| `PlateSize` | mm (W × H) | Offset on bed (mm) | Use |
|---|---|---|---|
| `SmallPlate` (0) | 85 × 55 | (97, 0) | 12-word seed only |
| `SquarePlate` (1) | 85 × 85 | (97, 49) | Seed + small descriptors |
| `LargePlate` (2) | 85 × 134 | (97, 0) | Full multisig descriptor backup |

Note the constant X-offset of **97 mm** in `offset()`
(`backup.go:49-56`). The engrave area starts 97 mm in from the origin on
every plate — that's the gap from the engraver's physical home to the
plate clamp. Y offset is 49 mm only for the square plate (the square
plate sits higher in the clamp).

### Safety margins

- `outerMargin = 3 mm` (`backup.go:79`) — minimum distance from any
  drawn pixel to the plate edge.
- `innerMargin = 10 mm` (`backup.go:80`) — clear region around the
  plate's mounting holes.

### Font / glyph format

The Pi-side controller **rasterises everything to Move/Line itself**.
The engraver knows nothing about glyphs, characters, or curves — it only
sees pen-up/pen-down moves to 24-bit coordinates.

From `font/font.go:11-78`:

- `font.Face` carries `Metrics{Ascent, Height float32}` plus an ASCII-only
  glyph index: `Index [unicode.MaxASCII]Glyph` — **only ASCII < 0x80 is
  supported**; non-ASCII runes return `false` from `Decode`.
- Each `Glyph` references a slice of `Segments []uint32` containing one
  of four opcodes followed by float32-bit-encoded coord pairs:
  - `SegmentOpMoveTo` (0) — 1 point
  - `SegmentOpLineTo` (1) — 1 point
  - `SegmentOpQuadTo` (2) — 2 points (control, endpoint)
  - `SegmentOpCubeTo` (3) — 3 points (two controls, endpoint)
- Three font faces are baked in at build time, all OpenType converted
  ahead of time by `font/convert.go`:
  - `font/comfortaa/`
  - `font/poppins/`
  - `font/constant/` (used by `engrave.ConstantStringer` for the
    constant-time seed/passphrase paths — see `engrave.go:610-790`).
- Quad and cubic Béziers are flattened to line segments inside
  `engrave.go` (search `SegmentOpQuadTo` / `SegmentOpCubeTo` ≈
  `engrave.go:1049-1057`) before being emitted as `p.Line(...)`.

So the wire never carries a glyph index. A composer that wants to emit
the same plates must either (a) ship its own font rasteriser producing
identical Move/Line streams, or (b) reuse the upstream `font` + `engrave`
packages directly.

## How v1 differs from v2 / SH2E (high-level)

v2 ("SH2E") is the wire format in `Gangleri42/seedhammer` on the
`seedhammer-features` branch under `engrave/wire/wire.go`. It is a
**different layer entirely**: a payload format, not a live machine
protocol.

| Aspect | v1 (this doc) | SH2E |
|---|---|---|
| What it describes | Live USB-serial protocol between Pi and engraver | A self-describing payload (text grid or curve set) carried out-of-band |
| Transport | USB-serial @ 115200 baud, command/response | NFC tag / NDEF record, MIME `application/vnd.seedhammer.engrave` |
| Frame | None — raw opcodes; phase implicit | 16-byte envelope: `'SH2E'` magic + version + mode + ptype + reserved + body length + CRC-32 |
| Integrity | None | CRC-32/IEEE over body |
| Versioning | None on the wire; firmware version baked into Pi controller | Version byte (currently `0x01`) |
| Coordinates | 24-bit LE per axis, in machine steps (1 step = 7.96 μm) | Knot streams (curves) or text-grid rows; abstract — engraver decides geometry |
| Content addressed | Move/Line raster only; controller has already chosen layout | Either `PtypeTextGrid` (16 lines × 26 chars max) or `PtypeCurves` (knots) |
| State | Stateful: init → setSpeed → setDelays → origin → warmup → program → end | Stateless: payload is fully self-describing |
| Cancellation | `0xAF` byte on serial | n/a — payload is delivered atomically |
| Constant-time modes | Done at the rasteriser layer (`engrave.ConstantStringer`, `engrave.ConstantQR`) — the engraver sees identical command counts for any seed | Explicit `ModeCT = 1` flag in envelope (and v1 firmware rejects it — see "v1 firmware rejected by SH2E" note in the wire summary) |
| Suitability for the composer | Composer must rasterise text/QR into Move/Line streams itself, then either drive the engraver directly OR emit a `.shp` file that mimics what `cmd/controller` would send | Composer can produce a high-level payload and let the engraver handle layout |

Practical implication for the v1 composer web app: we cannot just emit
"the v1 wire format" the way SH2E lets you emit an envelope. We need a
**rasteriser + plate layout engine** that produces a Move/Line stream
identical to what `backup.Engrave(...)` would produce in the Pi
controller. The Move/Line stream itself is then trivially serialisable
(it's just `(opcode, x, y)` triples). Easiest paths:

1. Port `engrave/engrave.go` + `font/font.go` + `backup/backup.go` to
   TS/Rust/whatever — they're pure functions, no hardware dependencies.
2. Or run the upstream Go packages headless from the composer backend
   (`engrave.Program` is an interface; supply a no-op `Move`/`Line` impl
   that just records calls).

## Open questions / things to verify on real hardware

1. **What is the third `setSpeedCmd` argument** (`xxx = 0xE6`)? The
   driver always passes 230, never anything else. Could be Z-axis speed,
   acceleration ramp, or a vestigial dwell time. Not documented anywhere
   in the code; needs a logic-analyser capture.
2. **Is Z (bytes 7-9 of a draw command) really always zero?** The
   simulator never reads it. If the MarkingWay firmware actually does
   anything with Z, we'd be silently ignoring it. Sniff the wire on a
   genuine v1 unit to confirm.
3. **`0x16` query-position**: the driver has the code but the result is
   thrown away (`_, _ = atleast, queryPos` at `driver.go:221`). Does the
   engraver actually respond to it? If yes, a composer could surface
   live position feedback.
4. **What happens if we send draw commands with `Z != 0`?** Could be
   useful for a deeper punch, or could panic the firmware. Don't try on
   anything that holds value until verified.
5. **Buffer depth.** `progBatchSize = 80` is fixed in the driver. Is 80
   a firmware-dictated maximum, or just a conservative number the
   controller chose? If we can raise it, big plates engrave with fewer
   round-trips.
6. **Plate origin assumption.** The driver assumes the user has jogged
   the needle to the plate's bottom-left before pressing engrave (the
   first `origin()` zeros the position there). The composer has no way
   to enforce this. Consider whether we add an explicit "home / jog"
   step in the composer-driven flow, or whether we keep the manual jog
   step from the SeedSigner-style UI.
7. **Cancellation race.** The driver sends `cancelCmd` then waits for
   `cancelledStatus`, but if a batch is mid-flight, can the engraver
   drop part of a 10-byte command and resync? Worth checking what the
   real firmware does — the sim assumes clean cancellation
   (`sim.go:147-148`).
8. **ASCII-only glyphs.** `font.Face.Index` is sized to
   `unicode.MaxASCII`. If the composer needs to emit BIP-39 in any
   language other than English, the font subsystem needs widening (or
   we restrict to English wordlists). Confirm whether v1 firmware has
   any opinion here — probably not, since it only sees Move/Line.
