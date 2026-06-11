/* SeedSigner sim — main-thread shell.
 * Owns the canvas + input; all Python runs in worker.js.
 * Button bit order must match _PIN_BITS in boot.py. */

const statusEl = document.getElementById("status");
const canvas = document.getElementById("lcd");
const ctx = canvas.getContext("2d");

if (!crossOriginIsolated) {
  statusEl.textContent =
    "Not cross-origin isolated — serve via tools/serve.py (COOP/COEP headers), " +
    "plain http.server won't work.";
  throw new Error("SharedArrayBuffer unavailable: missing COOP/COEP headers");
}

const inputSAB = new SharedArrayBuffer(4);
const sleepSAB = new SharedArrayBuffer(4);
const input = new Int32Array(inputSAB);

const worker = new Worker("./worker.js");
worker.postMessage({ type: "init", inputSAB, sleepSAB });

worker.onmessage = (ev) => {
  const m = ev.data;
  if (m.type === "frame") {
    if (canvas.width !== m.w || canvas.height !== m.h) {
      canvas.width = m.w;
      canvas.height = m.h;
    }
    ctx.putImageData(
      new ImageData(new Uint8ClampedArray(m.rgba.buffer), m.w, m.h), 0, 0);
    if (statusEl.dataset.running !== "1") {
      statusEl.dataset.running = "1";
      statusEl.textContent = "Running";
    }
  } else if (m.type === "status") {
    statusEl.dataset.running = "0";
    statusEl.textContent = m.text;
  } else if (m.type === "log") {
    console.log("[seedsigner]", m.text);
  } else if (m.type === "fatal") {
    statusEl.dataset.running = "0";
    statusEl.textContent = "Crashed — see browser console";
    console.error(m.text);
  }
};

/* bit order: 0 up, 1 down, 2 left, 3 right, 4 center/press, 5..7 keys 1..3 */
const KEYBITS = {
  ArrowUp: 0, ArrowDown: 1, ArrowLeft: 2, ArrowRight: 3,
  Enter: 4, " ": 4, "1": 5, "2": 6, "3": 7,
};

function setBit(bit, on) {
  if (on) Atomics.or(input, 0, 1 << bit);
  else Atomics.and(input, 0, ~(1 << bit));
}

window.addEventListener("keydown", (e) => {
  const bit = KEYBITS[e.key];
  if (bit !== undefined && !e.metaKey && !e.ctrlKey) {
    e.preventDefault();
    setBit(bit, true);
  }
});
window.addEventListener("keyup", (e) => {
  const bit = KEYBITS[e.key];
  if (bit !== undefined) {
    e.preventDefault();
    setBit(bit, false);
  }
});

/* On-screen joystick + keys (same data-btn bit numbering as the v1 emulator) */
document.querySelectorAll("[data-btn]").forEach((el) => {
  const bit = parseInt(el.dataset.btn, 10);
  const down = (e) => { e.preventDefault(); setBit(bit, true); };
  const up = (e) => { e.preventDefault(); setBit(bit, false); };
  el.addEventListener("pointerdown", down);
  el.addEventListener("pointerup", up);
  el.addEventListener("pointercancel", up);
  el.addEventListener("pointerleave", up);
});
