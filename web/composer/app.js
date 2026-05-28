// SeedHammer v1 composer — DOM shell + WASM bridge.
//
// The Go runtime (built from cmd/composer) exports a small surface:
//
//   composerVersion()            -> string   semver-ish identifier
//   composerPlateTypes()         -> {id, name, w_mm, h_mm}[]
//   composerEncodeText(plateType:number, lines:string[]) -> Uint8Array
//
// More exports (preview SVG, QR rendering, SVG mode) will follow.

const els = {
  status: document.getElementById("status"),
  plateTypes: document.getElementById("plate-types"),
  lines: document.getElementById("lines"),
  output: document.getElementById("output"),
  size: document.getElementById("size-meter"),
  btnGenerate: document.getElementById("btn-generate"),
};

const NUM_LINES = 12;

let wasmReady = false;
let plateType = 0; // default SmallPlate

function setStatus(text, error = false) {
  els.status.textContent = text;
  els.status.classList.toggle("error", error);
}

function buildPlateChoices(types) {
  els.plateTypes.innerHTML = "";
  for (const t of types) {
    const id = `plate-${t.id}`;
    const wrap = document.createElement("label");
    wrap.className = "plate-choice";
    wrap.innerHTML = `
      <input type="radio" name="plate-type" id="${id}" value="${t.id}" ${t.id === plateType ? "checked" : ""}>
      <span><strong>${t.name}</strong> <small>${t.w_mm} × ${t.h_mm} mm</small></span>
    `;
    wrap.querySelector("input").addEventListener("change", (e) => {
      plateType = Number(e.target.value);
    });
    els.plateTypes.appendChild(wrap);
  }
}

function buildLineInputs() {
  els.lines.innerHTML = "";
  for (let i = 0; i < NUM_LINES; i++) {
    const li = document.createElement("li");
    const input = document.createElement("input");
    input.type = "text";
    input.maxLength = 26;
    input.placeholder = i === 0 ? "First line, e.g. word1 word2 ..." : "";
    input.autocomplete = "off";
    input.spellcheck = false;
    li.appendChild(input);
    els.lines.appendChild(li);
  }
}

function readLines() {
  return [...els.lines.querySelectorAll("input")]
    .map((el) => el.value.toUpperCase().trim())
    .filter((s) => s.length > 0);
}

function showBytes(bytes) {
  els.size.textContent = `${bytes.length.toLocaleString("en-US")} B`;
  els.output.hidden = false;
  els.output.classList.remove("error");
  const hex = [...bytes].map((b) => b.toString(16).padStart(2, "0")).join(" ");
  els.output.textContent = `SH1E envelope (${bytes.length} bytes):\n\n${hex}`;
}

function showError(msg) {
  els.output.hidden = false;
  els.output.classList.add("error");
  els.output.textContent = msg;
}

function onGenerate() {
  if (!wasmReady) return;
  const lines = readLines();
  if (lines.length === 0) {
    showError("Enter at least one line of text.");
    return;
  }
  try {
    const bytes = globalThis.composerEncodeText(plateType, lines);
    showBytes(bytes);
  } catch (e) {
    showError(`Encode failed: ${e?.message ?? e}`);
  }
}

async function loadWasm() {
  setStatus("Loading WASM…");
  const go = new Go();
  const resp = await fetch("./composer.wasm");
  if (!resp.ok) {
    setStatus(`Failed to fetch composer.wasm (${resp.status})`, true);
    return;
  }
  const result = await WebAssembly.instantiateStreaming(resp, go.importObject);
  go.run(result.instance); // doesn't block — fires off the runtime goroutine
  // The Go main() blocks on a `select {}`, so the runtime keeps exports alive.
  // Spin until at least composerVersion is defined.
  for (let i = 0; i < 100; i++) {
    if (typeof globalThis.composerVersion === "function") break;
    await new Promise((r) => setTimeout(r, 20));
  }
  if (typeof globalThis.composerVersion !== "function") {
    setStatus("WASM loaded but exports never appeared", true);
    return;
  }
  const v = globalThis.composerVersion();
  const types = globalThis.composerPlateTypes();
  buildPlateChoices(types);
  buildLineInputs();
  wasmReady = true;
  els.btnGenerate.disabled = false;
  setStatus(`Ready — ${v}`);
}

els.btnGenerate.addEventListener("click", onGenerate);

loadWasm().catch((e) => {
  setStatus(`Boot failed: ${e?.message ?? e}`, true);
});
