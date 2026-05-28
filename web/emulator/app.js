// SeedHammer v1 emulator — DOM shell + WASM bridge.
//
// Go exports (built from cmd/emulator):
//
//   emulatorVersion()                          -> string
//   emulatorLCDSize                            -> {w, h}
//   emulatorPushEvent(buttonId, pressed)       -> void
//   emulatorBootScreen()                       -> void  (debug helper)
//
// The Go runtime calls back into JS via:
//   globalThis.emulatorPaint(pixels: Uint8ClampedArray, w: number, h: number)
// every time the firmware redraws the LCD.

const els = {
  status: document.getElementById("status"),
  lcd: document.getElementById("lcd"),
  buttons: document.querySelectorAll(".emu-btn"),
};

const ctx = els.lcd.getContext("2d", { alpha: false });

// Button enum mirrors platform/v1.Button.
const BTN = {
  Up: 0, Down: 1, Left: 2, Right: 3, Center: 4,
  Button1: 5, Button2: 6, Button3: 7,
};

const KEYMAP = {
  ArrowUp:    BTN.Up,
  ArrowDown:  BTN.Down,
  ArrowLeft:  BTN.Left,
  ArrowRight: BTN.Right,
  Enter:      BTN.Center,
  " ":        BTN.Center, // spacebar
  "1":        BTN.Button1,
  "2":        BTN.Button2,
  "3":        BTN.Button3,
};

let wasmReady = false;

function setStatus(text, error = false) {
  els.status.textContent = text;
  els.status.classList.toggle("error", error);
}

function pushEvent(id, pressed) {
  if (!wasmReady) return;
  globalThis.emulatorPushEvent(id, pressed);
}

// emulatorPaint: called from Go after every Display(). pixels is an
// RGBA byte buffer w*h*4 bytes long, row-major, top-left origin.
globalThis.emulatorPaint = function (pixels, w, h) {
  const img = new ImageData(pixels, w, h);
  // Resize canvas if Go reports a different LCD resolution.
  if (els.lcd.width !== w || els.lcd.height !== h) {
    els.lcd.width = w;
    els.lcd.height = h;
  }
  ctx.putImageData(img, 0, 0);
};

// Wire up the on-screen buttons. Pointerdown/up gives us press+release
// semantics that match the underlying GPIO event model.
for (const b of els.buttons) {
  const id = Number(b.dataset.btn);
  b.addEventListener("pointerdown", (e) => {
    e.preventDefault();
    b.classList.add("pressed");
    pushEvent(id, true);
  });
  const release = (e) => {
    e?.preventDefault();
    if (!b.classList.contains("pressed")) return;
    b.classList.remove("pressed");
    pushEvent(id, false);
  };
  b.addEventListener("pointerup", release);
  b.addEventListener("pointerleave", release);
  b.addEventListener("pointercancel", release);
  // Stop the button itself from grabbing keyboard focus on click.
  b.addEventListener("mousedown", (e) => e.preventDefault());
}

// Track currently-pressed keys so a held key doesn't repeatedly fire
// press events on autorepeat.
const heldKeys = new Set();

document.addEventListener("keydown", (e) => {
  const id = KEYMAP[e.key];
  if (id === undefined) return;
  if (heldKeys.has(e.key)) return; // autorepeat
  heldKeys.add(e.key);
  e.preventDefault();
  // Visual feedback on the on-screen button too.
  const dom = document.querySelector(`.emu-btn[data-btn="${id}"]`);
  if (dom) dom.classList.add("pressed");
  pushEvent(id, true);
});

document.addEventListener("keyup", (e) => {
  const id = KEYMAP[e.key];
  if (id === undefined) return;
  heldKeys.delete(e.key);
  e.preventDefault();
  const dom = document.querySelector(`.emu-btn[data-btn="${id}"]`);
  if (dom) dom.classList.remove("pressed");
  pushEvent(id, false);
});

async function loadWasm() {
  setStatus("Loading WASM…");
  const go = new Go();
  const resp = await fetch("./emulator.wasm");
  if (!resp.ok) {
    setStatus(`Failed to fetch emulator.wasm (${resp.status})`, true);
    return;
  }
  const result = await WebAssembly.instantiateStreaming(resp, go.importObject);
  go.run(result.instance);
  for (let i = 0; i < 100; i++) {
    if (typeof globalThis.emulatorVersion === "function") break;
    await new Promise((r) => setTimeout(r, 20));
  }
  if (typeof globalThis.emulatorVersion !== "function") {
    setStatus("WASM loaded but exports never appeared", true);
    return;
  }
  wasmReady = true;
  setStatus(`Ready — ${globalThis.emulatorVersion()}`);
}

loadWasm().catch((e) => {
  setStatus(`Boot failed: ${e?.message ?? e}`, true);
});
