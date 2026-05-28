# SeedHammer v1 — buttons + UI flow

Source pinned to upstream tag `v1.0.0` of `github.com/seedhammer/seedhammer`
(commit `6f9aa7a`, dated 2023-06-29). All file references are to that tag.

## Hardware

- **Display:** Waveshare 1.3" 240×240 LCD HAT (ST7789-based). Confirmed in the
  package comment of `input/input.go:1-2`:
  > "package input implements an input driver for the joystick and buttons on
  > the Waveshare 1.3" 240x240 HAT."
- **Camera:** OV5647 (per existing project knowledge — `cmd/controller/main.go`
  comment notes "in the same configuration as SeedSigner", which uses OV5647).
- **Pi board:** Raspberry Pi Zero (per `cmd/controller/main.go:1-2` —
  "It runs on a Raspberry Pi Zero, in the same configuration as SeedSigner.").
  SeedSigner v1 used Pi Zero v1.3 (camera-cable variant); SeedHammer ships the
  same SKU.
- **Buttons:** 8 physical inputs total = 5-way joystick + 3 keys. GPIO mapping
  confirmed from both the SeedHammer source and the Waveshare wiki — they match
  exactly. See table below.

## Physical layout (ASCII sketch)

Looking at the HAT mounted on top of the Pi, with the 240×240 LCD facing the
operator: the joystick sits at the lower-left corner and the three keys form a
vertical column on the lower-right.

```text
+-----------------------------------+
|                                   |
|                                   |
|         240x240 LCD               |
|                                   |
|                                   |
|                                   |
|                                   |
|        ^                          |
|        |                          |
|   <-- (o) -->        [KEY1]       |
|        |             [KEY2]       |
|        v             [KEY3]       |
|                                   |
+-----------------------------------+
```

The joystick is a 5-way: up, down, left, right, press-in (Center). KEY1/KEY2/KEY3
are momentary tactile switches. All eight inputs are active-low with pull-ups
enabled in firmware (`input/input.go:53` — `btn.Pin.In(gpio.PullUp, gpio.BothEdges)`).

## Code-level button names

All eight inputs are exposed as constants of type `input.Button`
(`input/input.go:19-31`). The order of the iota matters because the controller's
debug `input <name>` command (`cmd/controller/debug.go:62-94`) and the GUI
code branch on these constants.

| Physical input        | Go identifier     | BCM GPIO | Source                       |
|-----------------------|-------------------|----------|------------------------------|
| Joystick Up           | `input.Up`        | 6        | `input/input.go:42`          |
| Joystick Down         | `input.Down`      | 19       | `input/input.go:43`          |
| Joystick Left         | `input.Left`      | 5        | `input/input.go:44`          |
| Joystick Right        | `input.Right`     | 26       | `input/input.go:45`          |
| Joystick Press        | `input.Center`    | 13       | `input/input.go:46`          |
| Key 1 (top)           | `input.Button1`   | 21       | `input/input.go:47`          |
| Key 2 (middle)        | `input.Button2`   | 20       | `input/input.go:48`          |
| Key 3 (bottom)        | `input.Button3`   | 16       | `input/input.go:49`          |
| (debug-only) Rune     | `input.Rune`      | —        | `input/input.go:29`          |
| (debug-only) Screenshot| `input.Screenshot`| —       | `input/input.go:30`          |

`Rune` and `Screenshot` are synthetic events emitted only from the debug build
(`cmd/controller/debug.go`); on real hardware only the eight physical inputs fire.

The Waveshare wiki's Pinout table at
<https://www.waveshare.com/wiki/1.3inch_LCD_HAT> matches this list 1:1 (KEY1=P21,
KEY2=P20, KEY3=P16, Up=P6, Down=P19, Left=P5, Right=P26, Press=P13), which means
the v1 firmware is using the HAT's stock wiring — no custom board, no jumpers.

Event delivery model (`input/input.go:33-77`): one goroutine per pin, 10 ms
debounce, sends `Event{Button, Pressed bool}` on a channel. The GUI layer adds
a derived `Click` flag (`Pressed=false` transition after a `Pressed=true`) —
that's what most screens key off via `e.Click`.

## Button-role conventions across the GUI

Reading every `switch e.Button` in `gui/gui.go` (and `input.Button*` references —
~50 of them), the v1 firmware uses a strikingly consistent role assignment:

- **Joystick Up/Down** — scroll a list, move a selection up/down a column,
  move keyboard cursor up/down a row.
- **Joystick Left/Right** — page navigation (Receive↔Change addresses,
  Singlesig↔Multisig on the main screen), and left/right cursor inside the
  on-screen keyboard.
- **Joystick Center** — synonym for `Button3` ("primary confirm") in most
  screens. Explicit examples:
  - `gui/gui.go:2096` — `case input.Center, input.Button3:` (keyboard rune select)
  - `gui/gui.go:1666` — `case input.Button2, input.Center:` (Confirm-Seed edit)
  - `gui/gui.go:2231` — `case input.Button3, input.Center:` (engrave next-step)
  - `gui/gui.go:2462` — `case input.Button3, input.Center:` (main screen select)
- **Button1 (top)** — Back / cancel. Renders with `assets.IconBack`.
- **Button2 (middle)** — Secondary action (Edit, Info, Flip-camera). On the
  Engrave screen it's the press-and-hold "dry run" arming key
  (`gui/gui.go:1241-1247`).
- **Button3 (bottom)** — Primary action / confirm / next. Renders with
  `assets.IconCheckmark` or `assets.IconRight`. Press-and-hold to engrave
  (`gui/gui.go:1248-1255`, `confirmDelay`).

These conventions are not declared in one place — they emerge from how
`layoutNavigation` is called with `Style: StyleSecondary` (B1, B2) vs
`Style: StylePrimary` (B3). The pattern is consistent enough that the emulator
can mirror it without per-screen overrides.

## Main UI screen flow (high-level)

Entry point is `cmd/controller/main.go`, which constructs `gui.NewApp(...)` and
loops on `a.Frame()` forever. The app owns a single `MainScreen`
(`gui/gui.go:2311-2335, 2674-2702`); every other screen is a transient child
mounted on the MainScreen's fields (`scanner`, `desc`, `seed`, `engrave`,
`warning`, etc.) and unmounted when its `Layout` returns done.

```text
                       boot
                         |
                         v
                  +--------------+
                  |  MainScreen  |  page = singleKey | multiKey
                  | (carousel)   |  Left/Right: change page
                  +--------------+  Center/B3:   Select()
                         |
                +--------+---------+
                |                  |
        page == singleKey   page == multiKey
                |                  |
                v                  v
        +-------------+    +---------------+
        |  SeedScreen |    |  ScanScreen   |  (camera + QR)
        |  (enter 12/ |    |  Scan wallet  |
        |  24 words)  |    |  output desc  |
        +-------------+    +---------------+
                |                  |
                |                  v
                |          +-------------------+
                |          | DescriptorScreen  |
                |          | (shows xpubs;     |
                |          |  loops over each  |
                |          |  signer's seed)   |
                |          +-------------------+
                |                  |
                |                  v
                |          +-------------------+
                |          |   SeedScreen      |  per-signer
                |          +-------------------+
                |                  |
                +---------+--------+
                          v
                  +-----------------+
                  |  EngraveScreen  |  step-by-step
                  | (Connect Mjolnir|  instructions; B3
                  |  → align → cut) |  hold-to-engrave;
                  +-----------------+  B2 hold = dry run;
                          |            B1 = back/cancel.
                          v
                       complete --> back to MainScreen
```

Screen-by-screen button table (only the screens an emulator user will see in
the first 5 minutes; QR-scan and shamir flows omitted):

| Screen                | Up/Down            | Left/Right        | Center / B3                  | B1 (Back)         | B2                  | Source                |
|-----------------------|--------------------|-------------------|------------------------------|-------------------|---------------------|-----------------------|
| `MainScreen`          | —                  | switch page       | confirm `Select()`           | —                 | —                   | `gui/gui.go:2456-2494`|
| `ChoiceScreen` (12/24)| change choice      | —                 | confirm choice               | back              | —                   | `gui/gui.go:~1585`    |
| `WordKeyboardScreen`  | cursor row         | cursor column     | type letter                  | back              | (delete word?)      | `gui/gui.go:2043-2099`|
| `SeedScreen` (confirm)| select word        | —                 | (B3) confirm seed            | back (or discard) | (B2/Center) edit    | `gui/gui.go:1653-1703`|
| `DescriptorScreen`    | —                  | —                 | (B3) proceed                 | (B1) back         | (B2) addresses      | `gui/gui.go:475-495`  |
| `AddressesScreen`     | scroll page        | Receive↔Change    | —                            | (B1) close        | —                   | `gui/gui.go:246-269`  |
| `ScanScreen`          | —                  | —                 | (B3) accept                  | (B1) back         | (B2) flip-camera    | `gui/gui.go:610-625`  |
| `EngraveScreen`       | —                  | —                 | (B3) hold-to-engrave         | (B1) prev/cancel  | (B2) hold for dry-run| `gui/gui.go:1227-1263`|
| `ConfirmWarningScreen`| —                  | —                 | (B3) hold to confirm         | (B1) decline      | —                   | `gui/gui.go:864-870`  |
| `ErrorScreen`         | —                  | —                 | (B3) dismiss                 | —                 | —                   | `gui/gui.go:794-805`  |

Two interaction nuances the emulator must replicate:

1. **Hold-to-confirm.** `EngraveScreen` and `ConfirmWarningScreen` distinguish
   `e.Pressed` (key down) from `e.Click` (full down-up cycle). They start a
   `confirmDelay` countdown on press and complete the action only if the key is
   still held when the timeout fires (`gui/gui.go:1248-1255`). A browser
   emulator must therefore expose press-down and press-up as separate events,
   not just keypress.
2. **Idle screensaver.** `App.Frame` (`gui/gui.go:2706-2717`) shows a screensaver
   after 3 min of no input and "eats" the first button press to wake. The
   emulator should mirror this (or at least not break it) to keep behaviour
   true.

## Proposed emulator keyboard mapping

The v1 hardware has exactly 8 buttons + 2 debug synthetics, all available on a
standard desktop keyboard:

| Browser key           | Maps to            | Notes                                       |
|-----------------------|--------------------|---------------------------------------------|
| `ArrowUp`             | `input.Up`         | Joystick up                                 |
| `ArrowDown`           | `input.Down`       | Joystick down                               |
| `ArrowLeft`           | `input.Left`       | Joystick left                               |
| `ArrowRight`          | `input.Right`      | Joystick right                              |
| `Enter`               | `input.Center`     | Joystick press-in. Most "confirm" is here.  |
| `1`                   | `input.Button1`    | Top key (Back / cancel)                     |
| `2`                   | `input.Button2`    | Middle key (Secondary / dry-run)            |
| `3`                   | `input.Button3`    | Bottom key (Primary confirm / hold-to-act)  |
| `s` (shift+S)         | `input.Screenshot` | Debug-only on hardware; useful in emulator  |
| typing a letter A–Z   | `input.Rune`       | Debug `runes` shortcut equivalent           |

Recommended secondary aliases (no conflicts):

- `Escape` → `input.Button1` (universal "back" muscle memory).
- `Space` → `input.Button3` (universal "confirm" muscle memory; works with
  hold-to-confirm naturally because keydown/keyup map cleanly).
- `w/a/s/d` → Up/Left/Down/Right (gamer convention; optional, not default).

The emulator must emit **down and up** events separately. Browser model:
`keydown` → `Pressed: true`, `keyup` → `Pressed: false`. The 10 ms hardware
debounce can be skipped in the emulator since the OS already debounces.
Auto-repeat must be suppressed for keys that drive hold-to-confirm
(`event.repeat` filter on `keydown`), otherwise Button3 will fire
`Pressed: true` repeatedly and the GUI's `confirm.Start(...)` will never settle.

## v2 / Gangleri42 reference mapping (for comparison)

Important finding: Gangleri42's `cmd/wasmemu/` is **not a v1 emulator**. From
`cmd/wasmemu/keyboard.go:11-21` and the visible `cmd/wasmemu/index.html` header
("SeedHammer II — firmware emulator", 480×320 canvas):

> "SeedHammer II is a touch device — this is the primary input path on real
> hardware (see processTouch in cmd/controller/platform_sh2.go). [...] Touch is
> the only navigation input; keyboard exists solely for the NFC-tap shortcut."

The wasmemu binds `seedhammerTouch(x, y, pressed)` (mouse on canvas) for
navigation, and binds digit keys `1`–`9`, `0`, `e`, `q`, `w` to **NFC-tap
payload shortcuts** — not to UI buttons. The full payload-key table from
`cmd/wasmemu/index.html`:

| Key | NFC payload                                              |
|-----|----------------------------------------------------------|
| 1   | BIP-39 12-word                                           |
| 2   | BIP-39 24-word                                           |
| 3   | P2WSH 2-of-3 multisig                                    |
| 4   | P2SH 2-of-3 multisig                                     |
| 5   | singlesig (bare xpub)                                    |
| 6   | BlueWallet JSON multisig                                 |
| 7   | NIP-19 nsec                                              |
| 8   | NIP-19 npub                                              |
| 9   | codex32 share A (2-of-3)                                 |
| 0   | codex32 unshared                                         |
| e   | CUSTOM block text                                        |
| q   | unknown format (rejection)                               |
| w   | compound Nostr (rejection)                               |

**Implication for our v1 emulator design:** there is no prior-art keyboard map
to copy. We get to pick the v1 scheme cleanly. The proposed mapping above
(arrow keys + Enter + 1/2/3) reuses the digit keys for button-press in a way
that **does not collide** with the v2 emu's NFC shortcuts — because v1 has no
NFC. If we later build a combined v1+v2 emulator, the keys 1/2/3 will need to
context-switch based on whether the focused device is v1 (UI button) or v2
(NFC payload). Easier solution: reserve a different family of keys (e.g.,
F1/F2/F3) for v1 buttons if the combined emu happens. For a v1-only emu, the
proposed mapping above is correct.

We could optionally borrow Gangleri42's `seedhammerSynthTapText` /
`seedhammerSynthTapNDEF` pattern (untyped JS bridge globals) to bolt on a
"paste a mnemonic" debug helper in the v1 emulator that auto-types into the
`WordKeyboardScreen` via `input.Rune` events — same mechanism v1 already uses in
`cmd/controller/debug.go` `runes` command.

## Open questions

- **Long-press semantics.** v1 firmware reads `confirmDelay` from the GUI
  package; we should pin the exact value (likely 1.5 s — see
  `gui/gui.go` reference to `confirmDelay`). The emulator needs to mirror it
  exactly or hold-to-confirm "feels off". Worth grepping for the const in a
  follow-up pass.
- **Screensaver.** Should the emulator implement the 3-min idle screensaver,
  or is it a distraction? Probably skip in the browser — most demos last <3 min
  and the "eat first wake-press" behaviour confuses screencasting.
- **Camera substitute.** v1 `ScanScreen` expects a live OV5647 frame. The
  emulator will need a stub camera (likely a canned QR-bytes injector, mirroring
  Gangleri42's `seedhammerSynthTap` but for QR not NFC). Out of scope for this
  doc; track separately.
- **Engraver substitute.** v1 `EngraveScreen` writes to a serial port spoken to
  the Mjolnir engraver. The debug build already wires `mjolnir.NewSimulator()`
  (`cmd/controller/debug.go:18-21`) — reuse this in the wasm build by tagging
  appropriately.
- **Letter input.** `WordKeyboardScreen` uses Up/Down/Left/Right to drive a
  4-row on-screen keyboard. Should the emulator also accept direct A-Z typing
  via `input.Rune` (already supported in debug builds), or should it force the
  user through the joystick to faithfully reproduce hardware UX? Suggest both:
  default to faithful joystick, expose a "type words" debug helper for
  productivity.
