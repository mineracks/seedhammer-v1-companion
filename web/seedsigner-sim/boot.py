# SeedSigner-in-Pyodide boot shim.
#
# Runs inside the Web Worker's Pyodide interpreter. Replaces every hardware
# touchpoint of the upstream firmware with browser equivalents, then starts
# the real Controller loop. The upstream Python is bundled verbatim at the
# SHA pinned in docs/architecture/BASELINES.md — this file is the ONLY place
# where the sim diverges from the firmware.
#
# JS bridge (registered as the `shsim` module by worker.js):
#   gpio_state() -> int   bitmask of currently-held buttons (see _PIN_BITS)
#   sleep_ms(ms)          true blocking sleep via Atomics.wait (no CPU burn)
#   blit(w, h, rgba)      push a full RGBA frame to the visible canvas
#   status(text)          progress line for the page header
#   get_profile() -> str  display_config value chosen on the page
#   camera_start()        ask the main thread to begin webcam frame delivery
#   camera_stop()         stop webcam frame delivery
#   camera_read() -> {w,h,data}|None   latest RGB frame from the camera SAB
#   decode_qr(w, h, rgba) -> bytes|None   jsQR decode of an RGBA image

import json
import os
import sys
import time
import types
from unittest.mock import MagicMock

import shsim

sys.path.insert(0, "/app")
os.chdir("/app")  # settings.json persists in MEMFS for the session


# ── time.sleep → Atomics.wait ────────────────────────────────────────────
# Pyodide's time.sleep busy-waits; the firmware's input loop polls at 10ms
# intervals forever, which would peg a CPU core. Atomics.wait gives a real
# blocking sleep inside a worker.
def _sleep(seconds):
    ms = int(seconds * 1000)
    if ms > 0:
        shsim.sleep_ms(ms)

time.sleep = _sleep


# ── device profile → settings.json (read by Settings at first boot) ─────
# Values are upstream's SettingsConstants display_config options:
# st7789_240x240 (Classic), st7789_320x240 (SeedSigner+),
# ili9341_320x240 (SeedSigner+ panel variant).
_profile = str(shsim.get_profile() or "st7789_240x240")
with open("settings.json", "w") as _f:
    json.dump({"display_config": _profile}, _f)


# ── RPi.GPIO fake backed by the shared keyboard bitmask ──────────────────
# Pin numbers are upstream's P1_REVISION==3 BOARD-mode map (buttons.py).
_PIN_BITS = {
    31: 0,  # KEY_UP
    35: 1,  # KEY_DOWN
    29: 2,  # KEY_LEFT
    37: 3,  # KEY_RIGHT
    33: 4,  # KEY_PRESS (joystick center)
    40: 5,  # KEY1
    38: 6,  # KEY2
    36: 7,  # KEY3
}

GPIO = types.ModuleType("RPi.GPIO")
GPIO.RPI_INFO = {"P1_REVISION": 3}
GPIO.BOARD, GPIO.BCM = 10, 11
GPIO.IN, GPIO.OUT = 1, 0
GPIO.PUD_UP, GPIO.PUD_DOWN = 22, 21
GPIO.LOW, GPIO.HIGH = 0, 1
GPIO.setmode = GPIO.setup = GPIO.cleanup = GPIO.setwarnings = lambda *a, **k: None

def _gpio_input(pin):
    bit = _PIN_BITS.get(pin)
    if bit is None:
        return GPIO.HIGH
    return GPIO.LOW if (int(shsim.gpio_state()) >> bit) & 1 else GPIO.HIGH

GPIO.input = _gpio_input

_rpi = types.ModuleType("RPi")
_rpi.GPIO = GPIO
sys.modules["RPi"] = _rpi
sys.modules["RPi.GPIO"] = GPIO

# Hardware with no browser equivalent. (Camera and pyzbar get real shims
# below; this is just the remainder of the screenshot generator's mock set.)
for _name in ("picamera", "picamera.array", "seedsigner.hardware.microsd"):
    sys.modules[_name] = MagicMock()


# ── pyzbar → jsQR ────────────────────────────────────────────────────────
# DecodeQR only uses pyzbar.decode(img, symbols=[QRCODE], binary=...) and
# reads .data off each result. jsQR's binaryData serves both text and
# binary (CompactSeedQR) payloads.
import numpy as _np
from pyodide.ffi import to_js

_pyzbar_mod = types.ModuleType("pyzbar")
_pyzbar_sub = types.ModuleType("pyzbar.pyzbar")


class ZBarSymbol:
    QRCODE = "QRCODE"


class _Decoded:
    __slots__ = ("data", "type")

    def __init__(self, data):
        self.data = data
        self.type = "QRCODE"


def _pyzbar_decode(image, symbols=None, binary=False):
    if hasattr(image, "convert"):  # PIL image
        rgba = image.convert("RGBA")
        w, h = rgba.size
        buf = rgba.tobytes()
    else:  # numpy (h, w, 3) RGB array from the camera shim
        h, w = image.shape[:2]
        rgba = _np.dstack([image, _np.full((h, w), 255, dtype=_np.uint8)])
        buf = rgba.tobytes()
    res = shsim.decode_qr(w, h, to_js(buf))
    if res is None:
        return []
    return [_Decoded(bytes(res.to_py()))]


_pyzbar_sub.decode = _pyzbar_decode
_pyzbar_sub.ZBarSymbol = ZBarSymbol
_pyzbar_mod.pyzbar = _pyzbar_sub
sys.modules["pyzbar"] = _pyzbar_mod
sys.modules["pyzbar.pyzbar"] = _pyzbar_sub


# ── camera → browser webcam (or dropped QR image) via SAB frames ────────
# Mirrors upstream Camera's API surface, including the private
# `_video_stream` attr that ScanScreen's decode loop pokes directly.
# Divergences: no 90° rotation (webcams are upright, the Pi cam isn't),
# and the LCD live preview is a JS-side <video> overlay because
# LivePreviewThread can't run (no threads in Pyodide).
from PIL import Image

_camera_mod = types.ModuleType("seedsigner.hardware.camera")


class CameraConnectionError(Exception):
    pass


class Camera:
    _instance = None

    @classmethod
    def get_instance(cls):
        if cls._instance is None:
            cls._instance = cls()
        return cls._instance

    def __init__(self):
        self._video_stream = None

    def _read_frame(self):
        fr = shsim.camera_read()
        if fr is None:
            return None
        w, h = int(fr.w), int(fr.h)
        data = fr.data.to_py()
        return _np.frombuffer(data, dtype=_np.uint8).reshape((h, w, 3))

    def start_video_stream_mode(self, resolution=(512, 384), framerate=12,
                                format="bgr"):
        shsim.camera_start()
        self._video_stream = self  # truthy sentinel; upstream stores PiVideoStream

    def read_video_stream(self, as_image=False):
        if not self._video_stream:
            raise Exception("Must call start_video_stream first.")
        arr = self._read_frame()
        if arr is None:
            time.sleep(0.05)  # don't let the decode loop spin while no frames
            return None
        time.sleep(0.02)  # pace decode at ≲30fps; webcam pumps ~10fps anyway
        if not as_image:
            return arr
        return Image.fromarray(arr, "RGB").convert("RGBA")

    def stop_video_stream_mode(self):
        self._video_stream = None
        shsim.camera_stop()

    def start_single_frame_mode(self, resolution=(720, 480)):
        shsim.camera_start()

    def capture_frame(self):
        arr = self._read_frame()
        if arr is None:
            raise CameraConnectionError()
        return Image.fromarray(arr, "RGB")

    def stop_single_frame_mode(self):
        shsim.camera_stop()


_camera_mod.Camera = Camera
_camera_mod.CameraConnectionError = CameraConnectionError
sys.modules["seedsigner.hardware.camera"] = _camera_mod


# ── threads ──────────────────────────────────────────────────────────────
# Pyodide cannot start Python threads. BackgroundImportThread MUST still
# run (Controller.storage busy-waits on the _storage it seeds) — inline is
# fine, it's a finite pre-import pass. Everything else (toast managers,
# LivePreviewThread, microsd watcher) is cosmetic; skip them.
from seedsigner.models import threads as _ss_threads

def _thread_start(self):
    if type(self).__name__ == "BackgroundImportThread":
        self.run()

_ss_threads.BaseThread.start = _thread_start


# ── display driver → JS canvas ───────────────────────────────────────────
from seedsigner.hardware.displays import display_driver as _dd


class CanvasDisplay(_dd.BaseDisplayDriver):
    """Replaces ST7789/ILI9341 SPI drivers; frames go to the page canvas."""

    def __init__(self, display_type, width, height):
        # width/height arrive as the landscape canvas dims from the
        # display_config string. Renderer expects natively-portrait
        # drivers (ili9341/ili9486) to report swapped width/height —
        # it un-swaps them when sizing its canvas.
        if display_type in (_dd.DISPLAY_TYPE__ILI9341, _dd.DISPLAY_TYPE__ILI9486):
            super().__init__(_width=height, _height=width)
        else:
            super().__init__(_width=width, _height=height)
        self.display_type = display_type
        self._fb = Image.new("RGB", (width, height))

    def show_image(self, image, x_start: int = 0, y_start: int = 0):
        if image.size == self._fb.size and not x_start and not y_start:
            self._fb = image if image.mode == "RGB" else image.convert("RGB")
        else:
            self._fb.paste(image, (x_start, y_start))
        rgba = self._fb.convert("RGBA").tobytes()
        shsim.blit(self._fb.width, self._fb.height, to_js(rgba))


def _instantiate_display_driver(cls, display_type=_dd.DISPLAY_TYPE__ST7789,
                                width=None, height=None):
    return CanvasDisplay(display_type, int(width or 240), int(height or 240))

_dd.DisplayDriverFactory.instantiate_display_driver = classmethod(
    _instantiate_display_driver)


# ── go ───────────────────────────────────────────────────────────────────
shsim.status("Starting SeedSigner controller…")

from seedsigner.controller import Controller

try:
    Controller.get_instance().start()
except BaseException:  # surface tracebacks in the browser console
    import traceback
    shsim.status("SeedSigner controller exited — see console")
    traceback.print_exc()
    raise
