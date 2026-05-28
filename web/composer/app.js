// SeedHammer v1 composer — DOM shell + WASM bridge.
//
// Go exports (built from cmd/composer):
//
//   composerVersion()                              -> string
//   composerPlateTypes()                           -> {id, name, w_mm, h_mm}[]
//   composerEncodeText(plateType, lines)           -> Uint8Array (SH1E)
//   composerPreviewText(plateType, lines)          -> string (inline SVG)

const els = {
  status: document.getElementById("status"),
  plateTypes: document.getElementById("plate-types"),
  lines: document.getElementById("lines"),
  preview: document.getElementById("preview"),
  size: document.getElementById("size-meter"),
  output: document.getElementById("output"),
  btnBytes: document.getElementById("btn-bytes"),
};

// Build inputs for the largest possible plate; hide rows that don't fit
// the currently-selected plate. Keeps user data when switching back.
const MAX_LINES_ANY_PLATE = 20;

let wasmReady = false;
let plateType = 0;
let plateInfo = []; // populated from Go: [{id, name, w_mm, h_mm, max_lines}]
let visibleLines = MAX_LINES_ANY_PLATE;

function setStatus(text, error = false) {
  els.status.textContent = text;
  els.status.classList.toggle("error", error);
}

function buildPlateChoices(types) {
  plateInfo = types;
  els.plateTypes.innerHTML = "";
  for (const t of types) {
    const id = `plate-${t.id}`;
    const wrap = document.createElement("label");
    wrap.className = "plate-choice";
    wrap.innerHTML = `
      <input type="radio" name="plate-type" id="${id}" value="${t.id}" ${t.id === plateType ? "checked" : ""}>
      <span><strong>${t.name}</strong> <small>${t.w_mm} × ${t.h_mm} mm — ${t.max_lines} lines</small></span>
    `;
    wrap.querySelector("input").addEventListener("change", (e) => {
      plateType = Number(e.target.value);
      applyPlate();
      refresh();
    });
    els.plateTypes.appendChild(wrap);
  }
}

function applyPlate() {
  const info = plateInfo[plateType];
  visibleLines = Math.min(info?.max_lines ?? MAX_LINES_ANY_PLATE, MAX_LINES_ANY_PLATE);
  const items = els.lines.querySelectorAll("li");
  items.forEach((li, i) => {
    li.style.display = i < visibleLines ? "" : "none";
  });
}

function buildLineInputs() {
  els.lines.innerHTML = "";
  for (let i = 0; i < MAX_LINES_ANY_PLATE; i++) {
    const li = document.createElement("li");
    const input = document.createElement("input");
    input.type = "text";
    input.maxLength = 26;
    input.placeholder = i === 0 ? "First line…" : "";
    input.autocomplete = "off";
    input.spellcheck = false;
    input.addEventListener("input", scheduleRefresh);
    li.appendChild(input);
    els.lines.appendChild(li);
  }
  applyPlate();
}

function readLines() {
  return [...els.lines.querySelectorAll("input")]
    .slice(0, visibleLines) // only what fits the current plate
    .map((el) => el.value.toUpperCase().trim())
    .filter((s) => s.length > 0);
}

let refreshTimer = null;
function scheduleRefresh() {
  // Debounce keystrokes; 80ms feels live but doesn't thrash the WASM.
  if (refreshTimer) clearTimeout(refreshTimer);
  refreshTimer = setTimeout(refresh, 80);
}

function refresh() {
  refreshTimer = null;
  if (!wasmReady) return;
  const lines = readLines();
  // Empty: render an empty plate so the geometry still shows.
  let bytes = null;
  if (lines.length > 0) {
    try {
      bytes = globalThis.composerEncodeText(plateType, lines);
    } catch (e) {
      // Show the error in the size meter; preview still renders structurally.
      els.size.textContent = `error: ${e?.message ?? e}`;
      els.size.classList.add("error");
      els.preview.innerHTML = globalThis.composerPreviewText(plateType, lines);
      return;
    }
  }
  let svg;
  try {
    svg = globalThis.composerPreviewText(plateType, lines);
  } catch (e) {
    els.preview.textContent = `preview error: ${e?.message ?? e}`;
    return;
  }
  els.preview.innerHTML = svg;
  els.size.classList.remove("error");
  if (bytes) {
    els.size.textContent = `${bytes.length.toLocaleString("en-US")} B`;
  } else {
    els.size.textContent = "— B";
  }
}

// hexDump formats bytes in a compact 8-bytes-per-row layout. Total ~33
// chars wide, fits comfortably inside the actions card on every viewport.
//   00:  53 48 31 45 01 4f 00 56  |SH1E.O.V|
//   08:  14 48 32 a3 01 02 02 82  |.H2.....|
function hexDump(bytes) {
  const COLS = 8;
  const out = [];
  const offWidth = Math.max(2, bytes.length.toString(16).length);
  for (let i = 0; i < bytes.length; i += COLS) {
    const slice = Array.from(bytes.slice(i, i + COLS));
    const offset = i.toString(16).padStart(offWidth, "0");
    const hex = slice
      .map((b) => b.toString(16).padStart(2, "0"))
      .join(" ")
      .padEnd(COLS * 3 - 1, " ");
    const ascii = slice
      .map((b) => (b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : "."))
      .join("")
      .padEnd(COLS, " ");
    out.push(`${offset}:  ${hex}  |${ascii}|`);
  }
  return out.join("\n");
}

function showBytes() {
  if (!wasmReady) return;
  const lines = readLines();
  if (lines.length === 0) {
    els.output.hidden = false;
    els.output.classList.add("error");
    els.output.textContent = "Enter at least one line of text first.";
    return;
  }
  try {
    const bytes = globalThis.composerEncodeText(plateType, lines);
    els.output.hidden = false;
    els.output.classList.remove("error");
    els.output.textContent = `SH1E envelope — ${bytes.length} bytes\n\n${hexDump(bytes)}`;
  } catch (e) {
    els.output.hidden = false;
    els.output.classList.add("error");
    els.output.textContent = `Encode failed: ${e?.message ?? e}`;
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
  go.run(result.instance);
  for (let i = 0; i < 100; i++) {
    if (typeof globalThis.composerVersion === "function") break;
    await new Promise((r) => setTimeout(r, 20));
  }
  if (typeof globalThis.composerVersion !== "function") {
    setStatus("WASM loaded but exports never appeared", true);
    return;
  }
  const v = globalThis.composerVersion();
  buildPlateChoices(globalThis.composerPlateTypes());
  buildLineInputs();
  wasmReady = true;
  els.btnBytes.disabled = false;
  setStatus(`Ready — ${v}`);
  refresh(); // initial empty-plate render
}

els.btnBytes.addEventListener("click", showBytes);

loadWasm().catch((e) => {
  setStatus(`Boot failed: ${e?.message ?? e}`, true);
});
