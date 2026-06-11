/* Web Worker hosting Pyodide + the upstream SeedSigner firmware.
 *
 * The firmware's main loop blocks forever (wait_for polls buttons), which is
 * why it lives in a worker: input arrives through a SharedArrayBuffer the
 * main thread writes into, and frames leave via postMessage transfers.
 * Requires cross-origin isolation (COOP/COEP) — serve with tools/serve.py.
 */

const PYODIDE_VERSION = "0.27.2";
const PYODIDE_CDN = `https://cdn.jsdelivr.net/pyodide/v${PYODIDE_VERSION}/full/`;

let inputState; // Int32Array[1]: bitmask of held buttons (bit order in boot.py)
let sleepArr;   // Int32Array[1]: never signalled; used for Atomics.wait timeouts

function status(text) {
  postMessage({ type: "status", text });
}

self.onmessage = async (ev) => {
  if (ev.data.type !== "init") return;
  inputState = new Int32Array(ev.data.inputSAB);
  sleepArr = new Int32Array(ev.data.sleepSAB);
  try {
    await boot();
  } catch (err) {
    postMessage({ type: "fatal", text: String((err && err.stack) || err) });
  }
};

async function boot() {
  status("Loading Pyodide…");
  importScripts(PYODIDE_CDN + "pyodide.js");
  const pyodide = await loadPyodide({ indexURL: PYODIDE_CDN });
  pyodide.setStdout({ batched: (s) => postMessage({ type: "log", text: s }) });
  pyodide.setStderr({ batched: (s) => postMessage({ type: "log", text: s }) });

  status("Loading Pillow + numpy…");
  await pyodide.loadPackage(["pillow", "numpy"]);

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
  });

  status("Booting SeedSigner firmware…");
  const bootPy = await (await fetch("boot.py")).text();
  // Blocks for the lifetime of the firmware — the controller loop never returns.
  await pyodide.runPythonAsync(bootPy);
}
