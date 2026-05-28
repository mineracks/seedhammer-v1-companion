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
  btnQR: document.getElementById("btn-qr"),
  qrOverlay: document.getElementById("qr-overlay"),
  qrCanvas: document.getElementById("qr-canvas"),
  qrInfo: document.getElementById("qr-info"),
  qrClose: document.getElementById("qr-close"),
  tabs: document.querySelectorAll(".tab"),
  editorText: document.getElementById("editor-text"),
  editorSVG: document.getElementById("editor-svg"),
  svgFile: document.getElementById("svg-file"),
  svgSummary: document.getElementById("svg-summary"),
  svgError: document.getElementById("svg-error"),
};

// Build inputs for the largest possible plate; hide rows that don't fit
// the currently-selected plate. Keeps user data when switching back.
const MAX_LINES_ANY_PLATE = 20;

let wasmReady = false;
let plateType = 0;
let plateInfo = []; // populated from Go: [{id, name, w_mm, h_mm, max_lines}]
let visibleLines = MAX_LINES_ANY_PLATE;
let mode = "text"; // "text" | "svg"
let svgPaths = []; // d-strings extracted from the uploaded SVG, if any

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
  // Clear any stale Show-SH1E-bytes error as soon as the user types again.
  if (!els.output.hidden && els.output.classList.contains("error")) {
    els.output.hidden = true;
    els.output.classList.remove("error");
    els.output.textContent = "";
  }
}

function refresh() {
  refreshTimer = null;
  if (!wasmReady) return;
  if (mode === "svg") {
    refreshSVGMode();
  } else {
    refreshTextMode();
  }
}

function refreshTextMode() {
  const lines = readLines();
  let bytes = null;
  if (lines.length > 0) {
    try {
      bytes = globalThis.composerEncodeText(plateType, lines);
    } catch (e) {
      els.size.textContent = `error: ${e?.message ?? e}`;
      els.size.classList.add("error");
      els.preview.innerHTML = globalThis.composerPreviewText(plateType, lines);
      return;
    }
  }
  els.preview.innerHTML = globalThis.composerPreviewText(plateType, lines);
  els.size.classList.remove("error");
  els.size.textContent = bytes ? `${bytes.length.toLocaleString("en-US")} B` : "— B";
}

function refreshSVGMode() {
  let bytes = null;
  if (svgPaths.length > 0) {
    try {
      bytes = globalThis.composerEncodeSVG(plateType, svgPaths);
    } catch (e) {
      els.size.textContent = `error: ${e?.message ?? e}`;
      els.size.classList.add("error");
      els.preview.innerHTML = globalThis.composerPreviewSVG(plateType, svgPaths);
      return;
    }
  }
  els.preview.innerHTML = globalThis.composerPreviewSVG(plateType, svgPaths);
  els.size.classList.remove("error");
  els.size.textContent = bytes ? `${bytes.length.toLocaleString("en-US")} B` : "— B";
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
  let bytes;
  try {
    if (mode === "svg") {
      if (svgPaths.length === 0) {
        els.output.hidden = false;
        els.output.classList.add("error");
        els.output.textContent = "Upload an SVG file first.";
        return;
      }
      bytes = globalThis.composerEncodeSVG(plateType, svgPaths);
    } else {
      const lines = readLines();
      if (lines.length === 0) {
        els.output.hidden = false;
        els.output.classList.add("error");
        els.output.textContent = "Enter at least one line of text first.";
        return;
      }
      bytes = globalThis.composerEncodeText(plateType, lines);
    }
    els.output.hidden = false;
    els.output.classList.remove("error");
    els.output.textContent = `SH1E envelope — ${bytes.length} bytes\n\n${hexDump(bytes)}`;
  } catch (e) {
    els.output.hidden = false;
    els.output.classList.add("error");
    els.output.textContent = `Encode failed: ${e?.message ?? e}`;
  }
}

function setMode(newMode) {
  if (newMode === mode) return;
  mode = newMode;
  els.tabs.forEach((t) => {
    const isActive = t.dataset.mode === mode;
    t.classList.toggle("active", isActive);
    t.setAttribute("aria-selected", isActive ? "true" : "false");
  });
  els.editorText.hidden = mode !== "text";
  els.editorSVG.hidden = mode !== "svg";
  refresh();
}

async function onSVGFile(ev) {
  const f = ev.target.files?.[0];
  if (!f) return;
  els.svgError.hidden = true;
  try {
    const text = await f.text();
    const doc = new DOMParser().parseFromString(text, "image/svg+xml");
    if (doc.querySelector("parsererror")) throw new Error("file isn't valid SVG");
    const ds = [...doc.querySelectorAll("path")]
      .map((p) => p.getAttribute("d"))
      .filter(Boolean);
    if (ds.length === 0) {
      throw new Error("no <path d=\"...\"> elements found — flatten shapes to paths in your editor first");
    }
    svgPaths = ds;
    els.svgSummary.textContent = `${f.name} — ${ds.length} path${ds.length === 1 ? "" : "s"}, ${ds.reduce((n, d) => n + d.length, 0)} chars total`;
    refresh();
  } catch (e) {
    els.svgError.hidden = false;
    els.svgError.textContent = String(e?.message ?? e);
    svgPaths = [];
    refresh();
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
  els.btnQR.disabled = false;
  setStatus(`Ready — ${v}`);
  refresh(); // initial empty-plate render
}

function showQR() {
  if (!wasmReady) return;
  let result;
  try {
    if (mode === "svg") {
      if (svgPaths.length === 0) {
        setStatus("Upload an SVG file first", true);
        return;
      }
      result = globalThis.composerQRSVG(plateType, svgPaths);
    } else {
      const lines = readLines();
      if (lines.length === 0) {
        setStatus("Enter at least one line first", true);
        return;
      }
      result = globalThis.composerQR(plateType, lines);
    }
    els.qrCanvas.innerHTML = result.svg;
    els.qrInfo.textContent = `${result.bytes} B SH1E → QR ${result.modules}×${result.modules}`;
    els.qrOverlay.hidden = false;
    els.qrOverlay.setAttribute("aria-hidden", "false");
    setStatus(`Ready — ${composerVersionString()}`);
  } catch (e) {
    setStatus(`QR encode failed: ${e?.message ?? e}`, true);
  }
}

function hideQR() {
  els.qrOverlay.hidden = true;
  els.qrOverlay.setAttribute("aria-hidden", "true");
  els.qrCanvas.innerHTML = "";
}

let _ver;
function composerVersionString() {
  if (!_ver) _ver = globalThis.composerVersion().replace(/^v/, "");
  return _ver;
}

els.btnBytes.addEventListener("click", showBytes);
els.btnQR.addEventListener("click", showQR);
els.qrClose.addEventListener("click", hideQR);
els.tabs.forEach((t) => t.addEventListener("click", () => setMode(t.dataset.mode)));
els.svgFile.addEventListener("change", onSVGFile);
els.qrOverlay.addEventListener("click", (e) => {
  // Click outside the inner card dismisses; click inside (e.g. on the QR
  // itself) does nothing.
  if (e.target === els.qrOverlay) hideQR();
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && !els.qrOverlay.hidden) hideQR();
});

loadWasm().catch((e) => {
  setStatus(`Boot failed: ${e?.message ?? e}`, true);
});
