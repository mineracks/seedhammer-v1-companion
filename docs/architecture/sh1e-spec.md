# SH1E — SeedHammer v1 Engrave envelope format

**Status:** draft v0.1, 2026-05-28
**Stability:** unstable — the format will change as we validate it against
real hardware. Pin the version byte; any breaking change bumps it.

SH1E is the on-the-wire format that ships a plate design from the
browser-side composer to a SeedHammer v1 Pi controller, intended primarily
for transport via QR code(s) scanned by the Pi's camera. Secondary
transport channels (USB stick, HTTP) work without modification.

The Pi side parses SH1E, runs the parsed design through its **existing
trusted rasteriser** (`engrave/` + `font/` + `backup/` packages), shows
the user a preview on the LCD, and waits for hold-to-confirm before
streaming the resulting 10-byte commands to the engraver via `mjolnir/`.

The composer side emits SH1E for transport AND uses the same Go code in
WASM to render an in-browser preview that matches what the Pi will produce
**by code-identity**, not by re-implementation. See the [project plan
doc](./BASELINES.md) for the broader architecture.

## Design goals

1. **Intent, not commands.** SH1E describes what to engrave, not how. The
   trusted Pi controller rasterises locally; the composer cannot directly
   drive the engraver bytes. This keeps the security boundary in the right
   place.
2. **Single-frame QR friendly.** A 24-word plate fits in one QR (≤ ~1100
   bytes for QR version 24 with low EC). Larger designs degrade to
   multi-frame BBQr or animated QR.
3. **Self-describing.** Magic + version + length + CRC32 — any reader can
   detect "this is SH1E", reject the wrong version cleanly, and detect
   corruption from bit-flips in the QR.
4. **Canonical encoding.** Same design → same bytes → same QR. Users who
   want to verify "the QR I'm scanning is the design I intended" can
   reproduce it.
5. **Easy to parse safely.** Deterministic CBOR with strict ranges. No
   recursive structures, no length-prefix tricks, no implicit defaults
   that could be spoofed.

## Envelope

```
+----------+----------+--------------+--------+-----------+
| magic    | version  | payload_len  | crc32  | payload   |
| 4 bytes  | 1 byte   | 2 bytes (LE) | 4 bytes| N bytes   |
| "SH1E"   |   0x01   |  0x0000-     |        |           |
|          |          |   0xFFFF     |        |           |
+----------+----------+--------------+--------+-----------+
```

- **`magic`** = ASCII `SH1E` (0x53 0x48 0x31 0x45). Constant.
- **`version`** = current `0x01`. Any other value rejected by readers.
- **`payload_len`** = exact byte count of `payload`, little-endian uint16.
  Max 65535 bytes. (Reality budget: a single-frame QR fits ~1500 bytes
  of binary data; we'd never approach the uint16 ceiling.)
- **`crc32`** = CRC-32/ISO-HDLC (poly `0xEDB88320`, init `0xFFFFFFFF`,
  refin/refout true, xorout `0xFFFFFFFF`) computed over the `payload`
  bytes only (NOT magic/version/length). Little-endian uint32.
- **`payload`** = CBOR-encoded `Design` (see below). RFC 8949 deterministic
  encoding only.

Total minimum overhead: **11 bytes** of envelope.

## Payload structure (Design)

The `Design` is a CBOR map. All fields are required unless marked
optional. Top-level keys are short integers (deterministic encoding rule:
integer keys sort before string keys; integer keys are encoded in their
smallest form).

```cbor
{
  1: <uint>          # plate_type — see "Plate types" below
  2: [<TextBlock>],  # text_blocks (array, len 0..32)
  3: [<SvgPath>],    # svg_paths (array, len 0..16, OPTIONAL — omit if empty)
  4: <bytes>(32)     # design_fingerprint — SHA-256 of canonical-encoded fields 1-3, used as a stable design ID
}
```

### Plate types (integer enum, matches `backup/backup.go` order)

| Value | Name | Dimensions (mm) | Source |
|---:|---|---|---|
| 0 | `SmallPlate` | 85 × 55 | `backup.SmallPlate` |
| 1 | `SquarePlate` | 85 × 85 | `backup.SquarePlate` |
| 2 | `LargePlate` | 85 × 134 | `backup.LargePlate` |

### TextBlock

```cbor
{
  1: <uint>       # font_id — see "Font IDs"
  2: <uint>       # size — point size, 1..200 (clamped Pi-side)
  3: <int>        # x_mm — millimetres from plate origin, signed int (origin offset handled Pi-side)
  4: <int>        # y_mm — millimetres from plate origin
  5: <uint>       # alignment — 0 Left | 1 Center | 2 Right
  6: <tstr>       # text — UTF-8, but Pi-side limited to ASCII (rejects non-ASCII at parse)
  7: <uint>       # rotation — 0, 90, 180, 270 degrees only (OPTIONAL, default 0)
}
```

**Text length cap:** 256 chars per block (UTF-8 bytes, before ASCII check).
**Combined design cap:** 32 text blocks max.

### Font IDs (matches upstream v1 fonts at the pinned baseline)

| Value | Name | Source path |
|---:|---|---|
| 0 | `Comfortaa` | `font/comfortaa` |
| 1 | `Poppins` | `font/poppins` |
| 2 | `Constant` | `font/constant` |

If we lift additional fonts in a later baseline, the enum extends but
**never renumbers**.

### SvgPath

For logos, custom marks, embedded designs. The Pi parses these to its
internal segment representation (`MoveTo` / `LineTo` / `QuadTo` /
`CubeTo`) — same shape as the font glyph code already handles.

```cbor
{
  1: <int>        # x_mm — anchor X
  2: <int>        # y_mm — anchor Y
  3: <uint>       # scale_pct — percentage 1..1000 (clamped Pi-side)
  4: <tstr>       # path_d — SVG `d` attribute string, validated against a strict subset
  5: <uint>       # rotation — 0/90/180/270 (OPTIONAL, default 0)
}
```

**SVG path subset (strict):**
- Commands allowed: `M`, `L`, `H`, `V`, `Q`, `C`, `Z` (and lowercase relatives `m`, `l`, `h`, `v`, `q`, `c`, `z`)
- No arcs (`A`/`a`) — they require curve approximation that's easier to do composer-side
- No `S`/`T` shortcuts — composer must emit explicit `Q`/`C` instead
- Coordinates: decimal numbers with optional leading sign, no scientific
  notation, no comma separators, single-space-separated. (Stricter than
  full SVG but trivially parseable in Go.)
- Max path length: 4096 chars
- Max number of points: 1024

## Canonical encoding rules

Per RFC 8949 § 4.2.1, with these additional clarifications:

1. Map keys sorted by integer value ascending.
2. Integers in their shortest CBOR form (tiny ints inline, then uint8,
   uint16, uint32, uint64 as needed).
3. Text strings encoded as UTF-8 with no BOM.
4. Byte strings have explicit length prefix.
5. No indefinite-length items.
6. No tags (no CBOR semantic tags used in this spec).
7. Floating point not used anywhere (all numerics are integers).

**Two encodings of the same Design MUST produce byte-identical payload.**
This is enforceable by the composer using a known-canonical CBOR library
(e.g. `fxamacker/cbor` with `EncOptions{Sort: SortBytewiseLexical}`).

## QR transport

A single SH1E envelope:

| Design complexity | Approx envelope size | Recommended QR |
|---|---:|---|
| 12-word seed, SmallPlate, single font block | ~150 B | QR version 6, M ECC, alphanumeric mode |
| 24-word seed, LargePlate, 2 font blocks | ~280 B | QR version 10, M ECC, byte mode |
| 24-word seed + multisig descriptor, LargePlate, 4 font blocks | ~500 B | QR version 17, M ECC, byte mode |
| Above + small SVG logo | ~700 B | QR version 22, M ECC |
| Multi-plate manifest (3 plates) | 1.5-2 KB | **BBQr or animated QR** |

For multi-frame transport, use **BBQr** (https://github.com/coinkite/BBQr)
as it's already supported by ColdCard / Sparrow / Specter and we'd get
ecosystem compatibility for free. Don't invent a new chunking format.

The Pi controller scans BBQr or single QR transparently — the camera
flow on v1 already understands both via its existing QR reader.

## Examples

### Minimal: 12-word seed on SmallPlate, single font block

```json5
{
  1: 0,           // plate_type = SmallPlate
  2: [
    {
      1: 0,       // font_id = Comfortaa
      2: 12,      // size = 12pt
      3: 5,       // x_mm
      4: 5,       // y_mm
      5: 0,       // alignment = Left
      6: "ABANDON ABILITY ABLE ABOUT ABOVE ABSENT ABSORB ABSTRACT ABSURD ABUSE ACCESS ACCIDENT"
    }
  ],
  4: <32-byte SHA-256>
}
```

CBOR encoding of this: approximately 130 bytes (depends on the exact
seed words used).

### Two-block layout with logo

```json5
{
  1: 1,           // plate_type = SquarePlate
  2: [
    {  // title
      1: 1,       // font_id = Poppins
      2: 18,
      3: 10, 4: 8,
      5: 1,       // Center
      6: "MY MULTISIG KEY 1 OF 3"
    },
    {  // seed words
      1: 0,       // font_id = Comfortaa
      2: 12,
      3: 5, 4: 25,
      5: 0,
      6: "WORD1 WORD2 WORD3 ... WORD24"
    }
  ],
  3: [
    {  // BTC logo top-right
      1: 70, 2: 5,
      3: 100,     // 100% scale
      4: "M 0 0 L 10 0 L 10 10 L 0 10 Z M 2 2 L 8 8 M 8 2 L 2 8"
    }
  ],
  4: <32-byte SHA-256>
}
```

## Parser validation rules (Pi side)

The Pi-side parser **must** reject a SH1E envelope if any of these hold,
without further processing:

1. `magic` ≠ `SH1E`
2. `version` ≠ `0x01`
3. `payload_len` doesn't match actual payload byte length
4. `crc32` doesn't match recomputed CRC
5. Payload is not canonical CBOR (sort order wrong, indefinite-length used,
   floats present, unknown tag present)
6. Required key missing (1, 2, or 4 absent)
7. Any unknown integer key in any map (forwards-compat: unknown string
   keys allowed and ignored, unknown integer keys MUST be rejected — this
   reserves int keys for future-version expansion)
8. `plate_type` not in `{0, 1, 2}`
9. `font_id` references a font not bundled in the running firmware
10. `text` contains a non-ASCII codepoint (because v1 fonts are ASCII-only)
11. Any numeric field outside its documented range
12. Total text block count > 32 or any text length > 256 bytes
13. Total SVG path count > 16 or any path string > 4096 chars
14. `design_fingerprint` doesn't match a recomputed SHA-256 of fields 1-3
15. Resulting plate layout exceeds plate dimensions (with the
    `outerMargin = 3 mm` from `backup.go` applied)
16. Total command stream would exceed the engraver's safe operating envelope

After validation passes, the Pi:
- Renders a preview on the LCD using the existing rasterisation pipeline
- Shows total command count + estimated engrave time
- Waits for hold-to-confirm on Button3 (the existing engrave-confirm pattern)
- Streams the resulting commands via `mjolnir/`

## Forward compatibility

Two extension dimensions:

1. **Bumping version byte** (`0x01 → 0x02`) — used for breaking changes
   (e.g. coordinate system change, new envelope structure). Old firmware
   rejects, prompting user to update.
2. **Adding new integer keys** to the maps — used for non-breaking
   additions (e.g. new optional `kerning` field on `TextBlock`). Old
   firmware **rejects** unknown integer keys (per rule 7 above) so a
   composer using v0.2 keys can target v0.1 readers only by omitting them.
3. **String keys** are reserved for **forward-compat hints** — old readers
   ignore them. Use sparingly and never for security-relevant data (since
   they're ignored).

## Security analysis

### Threat model

The Pi controller is the trusted compute base. The composer is untrusted —
could be malicious, could have bugs, could be served from a tampered CDN.

### What SH1E protects against

- **Wrong content engraved silently:** the Pi rasterises locally and shows
  a preview the user must confirm. A malicious composer cannot bypass.
- **Out-of-range coordinates damaging the machine:** parser rule 15 +
  range checks on every numeric.
- **Buffer-overflow style attacks:** strict size caps on all variable-length
  fields, CBOR parser used must be vetted for buffer safety.
- **Confused-deputy via SVG complexity:** strict subset, hard caps on
  path string length and point count.
- **Bit-flip corruption in QR:** CRC32 catches single-byte flips with
  ~99.9999998% probability.

### What SH1E does NOT protect against

- **Social engineering** to engrave the wrong seed. The Pi preview is
  the user's last line of defence. They must read it carefully.
- **Pre-image or collision attacks on `design_fingerprint`.** It's
  SHA-256 over canonical bytes — strong enough that any practical
  collision is computationally infeasible.
- **Malicious upstream firmware.** Out of scope — that's a wider problem.

### Fuzzing requirement

Before any release ships with SH1E support, the Pi-side parser MUST be
fuzz-tested. Recommend `go-fuzz` or Go 1.18+ native fuzzing for at least
1 CPU-week against the parser entry point.

## Open questions

1. **Should we use `engrave/wire/`-style envelope for binary symmetry with
   SH2E?** Currently no — SH2E is a *post-rasterised command stream*, SH1E
   is a *design intent*. They serve different purposes. But the magic
   prefix `SH1E` parallels `SH2E` deliberately.
2. **Should the Pi sign acknowledgement of a successful engrave back via
   QR?** Useful for multi-plate flows (composer can verify which plate
   was actually written). Out of scope for v0.1 — add in v0.2 if there's
   user demand.
3. **What about hardware-bound designs?** A user could optionally embed
   the iDRAC/serial number of their specific SeedHammer v1 in the design
   to prevent "wrong physical device played the wrong design". Probably
   overkill. Out of scope for v0.1.
4. **Should we support custom fonts uploaded as part of the design?** No.
   That dramatically expands the attack surface (font parsers are
   notoriously bug-prone) and breaks the "rasterise with trusted Pi
   code" property. If users want a new font, propose upstream and we
   ship it in firmware.

## Implementation references

- CBOR library (Pi side, Go): `github.com/fxamacker/cbor/v2` — already
  used elsewhere in the seedhammer codebase
- CBOR library (composer side, Go-WASM): same package, compiled to WASM
- CRC32 (Go): `hash/crc32.MakeTable(crc32.IEEE)` — standard library
- BBQr (Go): port from `github.com/coinkite/BBQr` Python reference, or
  use an existing Go BBQr lib if one exists at implementation time

## Sources

- Upstream `backup/backup.go` for plate dimensions (pinned in
  [BASELINES.md](./BASELINES.md))
- Upstream `mjolnir/driver.go` for engraver wire details (see
  [v1-engrave-spec.md](./v1-engrave-spec.md))
- RFC 8949 (CBOR)
- RFC 1952 (CRC-32 variant matches; see also CRC catalogue at
  reveng.sourceforge.io for `CRC-32/ISO-HDLC`)
- BBQr spec at https://github.com/coinkite/BBQr
