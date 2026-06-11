/* SeedSigner sim — main-thread shell.
 * Owns the canvas, input, webcam pump, drag-drop QR injection, device
 * profile selection, and the QR handoff to the SeedHammer v1 emulator.
 * All Python runs in worker.js. Button bit order must match boot.py. */

const statusEl = document.getElementById("status");
const canvas = document.getElementById("lcd");
const ctx = canvas.getContext("2d", { willReadFrequently: true });
const lcdWrap = document.querySelector(".sss-lcd-screen");
const camVideo = document.getElementById("campreview");
const profileSel = document.getElementById("profile");
const handoffBtn = document.getElementById("handoff");

if (!crossOriginIsolated) {
  statusEl.textContent =
    "Not cross-origin isolated — serve via tools/serve.py (COOP/COEP headers), " +
    "plain http.server won't work.";
  throw new Error("SharedArrayBuffer unavailable: missing COOP/COEP headers");
}

/* ─── device profile ───────────────────────────────────────────────── */

const PROFILE_KEY = "sss-profile";
const profile = localStorage.getItem(PROFILE_KEY) || "st7789_240x240";
profileSel.value = profile;
profileSel.addEventListener("change", () => {
  localStorage.setItem(PROFILE_KEY, profileSel.value);
  location.reload(); // firmware reads the profile once at boot
});

/* ─── worker + shared memory ───────────────────────────────────────── */

const CAM_W_MAX = 640, CAM_H_MAX = 480, CAM_HEADER_BYTES = 16;

const inputSAB = new SharedArrayBuffer(4);
const sleepSAB = new SharedArrayBuffer(4);
const camSAB = new SharedArrayBuffer(CAM_HEADER_BYTES + CAM_W_MAX * CAM_H_MAX * 3);
const input = new Int32Array(inputSAB);
const camHeader = new Int32Array(camSAB, 0, 4);
const camPixels = new Uint8Array(camSAB, CAM_HEADER_BYTES);

const worker = new Worker("./worker.js");
worker.postMessage({ type: "init", inputSAB, sleepSAB, camSAB, profile });

worker.onmessage = (ev) => {
  const m = ev.data;
  if (m.type === "frame") {
    if (canvas.width !== m.w || canvas.height !== m.h) {
      canvas.width = m.w;
      canvas.height = m.h;
      canvas.style.width = `${Math.round(m.w * 1.2)}px`;
      canvas.style.height = `${Math.round(m.h * 1.2)}px`;
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
  } else if (m.type === "camera") {
    m.on ? cameraOn() : cameraOff();
  } else if (m.type === "log") {
    console.log("[seedsigner]", m.text);
  } else if (m.type === "fatal") {
    statusEl.dataset.running = "0";
    statusEl.textContent = "Crashed — see browser console";
    console.error(m.text);
  }
};

/* ─── buttons / keyboard ───────────────────────────────────────────── */
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

document.querySelectorAll("[data-btn]").forEach((el) => {
  const bit = parseInt(el.dataset.btn, 10);
  const down = (e) => { e.preventDefault(); setBit(bit, true); };
  const up = (e) => { e.preventDefault(); setBit(bit, false); };
  el.addEventListener("pointerdown", down);
  el.addEventListener("pointerup", up);
  el.addEventListener("pointercancel", up);
  el.addEventListener("pointerleave", up);
});

/* ─── camera pump ──────────────────────────────────────────────────── */
/* The worker can't receive messages while Python runs, so frames go
 * through camSAB: [seq, w, h, _] header + RGB bytes. The firmware's
 * LivePreviewThread can't run (no threads in Pyodide), so we overlay the
 * real <video> on the LCD for visual feedback while scanning. */

let camStream = null;
let camTimer = null;
let dropHoldUntil = 0; // pause webcam writes so a dropped image survives
const pumpCanvas = document.createElement("canvas");
const pumpCtx = pumpCanvas.getContext("2d", { willReadFrequently: true });

function writeCameraFrame(source, sw, sh) {
  let w = sw, h = sh;
  const scale = Math.min(CAM_W_MAX / w, CAM_H_MAX / h, 1);
  w = Math.round(w * scale);
  h = Math.round(h * scale);
  pumpCanvas.width = w;
  pumpCanvas.height = h;
  pumpCtx.drawImage(source, 0, 0, w, h);
  const data = pumpCtx.getImageData(0, 0, w, h).data;
  for (let i = 0, o = 0; o < w * h * 3; i += 4, o += 3) {
    camPixels[o] = data[i];
    camPixels[o + 1] = data[i + 1];
    camPixels[o + 2] = data[i + 2];
  }
  Atomics.store(camHeader, 1, w);
  Atomics.store(camHeader, 2, h);
  Atomics.add(camHeader, 0, 1); // seq++ — frame visible to the worker
}

async function cameraOn() {
  lcdWrap.classList.add("cam-on");
  if (camStream) return;
  try {
    camStream = await navigator.mediaDevices.getUserMedia({
      video: { width: { ideal: 640 }, height: { ideal: 480 }, facingMode: "environment" },
      audio: false,
    });
  } catch (e) {
    console.warn("webcam unavailable:", e);
    statusEl.textContent =
      "Webcam unavailable — drop a QR image onto the device screen instead.";
    return;
  }
  camVideo.srcObject = camStream;
  await camVideo.play().catch(() => {});
  camTimer = setInterval(() => {
    if (Date.now() < dropHoldUntil) return;
    if (camVideo.videoWidth) {
      writeCameraFrame(camVideo, camVideo.videoWidth, camVideo.videoHeight);
    }
  }, 100);
}

function cameraOff() {
  lcdWrap.classList.remove("cam-on");
  if (camTimer) { clearInterval(camTimer); camTimer = null; }
  if (camStream) {
    camStream.getTracks().forEach((t) => t.stop());
    camStream = null;
    camVideo.srcObject = null;
  }
  Atomics.store(camHeader, 0, 0); // no frames while camera is off
}

/* Drag-drop a QR image anywhere on the device card — works without a
 * webcam and is what the e2e tests use. */
const deviceCard = document.querySelector(".sss-device-card");
deviceCard.addEventListener("dragover", (e) => e.preventDefault());
deviceCard.addEventListener("drop", async (e) => {
  e.preventDefault();
  const file = e.dataTransfer?.files?.[0];
  if (!file || !file.type.startsWith("image/")) return;
  const bmp = await createImageBitmap(file);
  injectCameraImage(bmp, bmp.width, bmp.height);
});

/* Also callable directly (tests, console): inject any drawable source. */
function injectCameraImage(source, w, h) {
  dropHoldUntil = Date.now() + 3000;
  writeCameraFrame(source, w, h);
}
window.sssInjectCameraImage = injectCameraImage;

/* ─── QR handoff → SeedHammer v1 emulator ──────────────────────────── */

function loadScript(src) {
  return new Promise((res, rej) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = res;
    s.onerror = rej;
    document.head.appendChild(s);
  });
}

async function ensureJsQR() {
  if (typeof jsQR === "function") return;
  try {
    await loadScript("./vendor/jsqr.js");
  } catch {
    await loadScript("https://cdn.jsdelivr.net/npm/jsqr@1.4.0/dist/jsQR.js");
  }
}

handoffBtn.addEventListener("click", async () => {
  await ensureJsQR();
  const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
  const res = jsQR(img.data, canvas.width, canvas.height,
    { inversionAttempts: "attemptBoth" });
  if (!res || !res.binaryData?.length) {
    statusEl.dataset.running = "0";
    statusEl.textContent =
      "No QR on the SeedSigner screen — export one first (e.g. Seeds → Export as SeedQR).";
    return;
  }
  const payload = btoa(String.fromCharCode(...res.binaryData));
  localStorage.setItem("sh1-handoff", JSON.stringify({
    payload,
    frame: canvas.toDataURL("image/png"),
  }));
  window.open("../emulator/?handoff=1", "_blank");
});
