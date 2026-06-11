/* Web Worker hosting Pyodide + the upstream SeedSigner firmware.
 *
 * The firmware's main loop blocks forever (wait_for polls buttons), which is
 * why it lives in a worker: input and camera frames arrive through
 * SharedArrayBuffers the main thread writes into (this worker's onmessage
 * never fires while Python runs), and LCD frames leave via postMessage
 * transfers. Requires cross-origin isolation — serve with tools/serve.py.
 *
 * Pyodide + jsQR load from ./vendor/ (run vendor-pyodide.sh); falls back to
 * the jsdelivr CDN when the vendor dir is absent.
 */

const PYODIDE_VERSION = "0.27.2";
const PYODIDE_CDN = `https://cdn.jsdelivr.net/pyodide/v${PYODIDE_VERSION}/full/`;
const PYODIDE_LOCAL = "./vendor/pyodide/";
const JSQR_LOCAL = "./vendor/jsqr.js";
const JSQR_CDN = "https://cdn.jsdelivr.net/npm/jsqr@1.4.0/dist/jsQR.js";

let inputState; // Int32Array[1]: bitmask of held buttons (bit order in boot.py)
let sleepArr;   // Int32Array[1]: never signalled; used for Atomics.wait timeouts
let camHeader;  // Int32Array[4] over camSAB: [seq, width, height, reserved]
let camSAB;     // SharedArrayBuffer: 16-byte header + RGB pixels
let profile = "st7789_240x240";

const CAM_HEADER_BYTES = 16;

function status(text) {
  postMessage({ type: "status", text });
}

async function exists(url) {
  try {
    return (await fetch(url, { method: "HEAD" })).ok;
  } catch {
    return false;
  }
}

self.onmessage = async (ev) => {
  if (ev.data.type !== "init") return;
  inputState = new Int32Array(ev.data.inputSAB);
  sleepArr = new Int32Array(ev.data.sleepSAB);
  camSAB = ev.data.camSAB;
  camHeader = new Int32Array(camSAB, 0, 4);
  profile = ev.data.profile || profile;
  try {
    await boot();
  } catch (err) {
    postMessage({ type: "fatal", text: String((err && err.stack) || err) });
  }
};

function cameraRead() {
  const seq = Atomics.load(camHeader, 0);
  if (!seq) return null;
  const w = Atomics.load(camHeader, 1);
  const h = Atomics.load(camHeader, 2);
  if (!w || !h) return null;
  // Copy out of the SAB so Python gets a stable snapshot.
  const px = new Uint8Array(camSAB, CAM_HEADER_BYTES, w * h * 3).slice();
  return { w, h, data: px };
}

async function boot() {
  const local = await exists(PYODIDE_LOCAL + "pyodide.js");
  const base = local ? PYODIDE_LOCAL : PYODIDE_CDN;
  if (!local) console.warn("pyodide vendor dir missing — using CDN (run vendor-pyodide.sh)");

  status(`Loading Pyodide (${local ? "local" : "CDN"})…`);
  importScripts(base + "pyodide.js");
  importScripts((await exists(JSQR_LOCAL)) ? JSQR_LOCAL : JSQR_CDN);

  const pyodide = await loadPyodide({ indexURL: base });
  pyodide.setStdout({ batched: (s) => postMessage({ type: "log", text: s }) });
  pyodide.setStderr({ batched: (s) => postMessage({ type: "log", text: s }) });

  status("Loading Pillow + numpy…");
  // "hashlib" is Pyodide's unvendored OpenSSL _hashlib — without it
  // hashlib.pbkdf2_hmac doesn't exist and embit's mnemonic_to_seed dies.
  await pyodide.loadPackage(["pillow", "numpy", "hashlib"], {
    messageCallback: () => {},
  });

  status("Fetching SeedSigner bundle…");
  const resp = await fetch("vendor/seedsigner-bundle.zip");
  if (!resp.ok) {
    throw new Error(
      `bundle fetch failed (${resp.status}) — run web/seedsigner-sim/build.sh`);
  }
  pyodide.unpackArchive(await resp.arrayBuffer(), "zip", { extractDir: "/app" });

  pyodide.registerJsModule("shsim", {
    gpio_state: () => Atomics.load(inputState, 0),
    // sleepArr[0] is always 0 and never notified, so this is a pure timeout.
    sleep_ms: (ms) => Atomics.wait(sleepArr, 0, 0, Math.max(1, ms)),
    blit: (w, h, rgba) => {
      // rgba is a fresh Uint8Array copy (to_js of Python bytes) — transferable.
      postMessage({ type: "frame", w, h, rgba }, [rgba.buffer]);
    },
    status,
    get_profile: () => profile,
    camera_start: () => postMessage({ type: "camera", on: true }),
    camera_stop: () => postMessage({ type: "camera", on: false }),
    camera_read: cameraRead,
    decode_qr: (w, h, rgba) => {
      const px = new Uint8ClampedArray(rgba.buffer, rgba.byteOffset, w * h * 4);
      const res = jsQR(px, w, h, { inversionAttempts: "attemptBoth" });
      return res && res.binaryData && res.binaryData.length
        ? new Uint8Array(res.binaryData)
        : null;
    },
  });

  status("Booting SeedSigner firmware…");
  const bootPy = await (await fetch("boot.py")).text();
  // Blocks for the lifetime of the firmware — the controller loop never returns.
  await pyodide.runPythonAsync(bootPy);
}
