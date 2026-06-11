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

# Hardware that has no browser equivalent (yet). Same set the upstream
# screenshot generator mocks. Camera/QR-scan gets a real shim later —
# browser-side decode feeding a fake camera (seedsigner-reuse.md, open q. 3).
for _name in (
    "picamera",
    "picamera.array",
    "pyzbar",
    "pyzbar.pyzbar",
    "seedsigner.hardware.microsd",
):
    sys.modules[_name] = MagicMock()


# ── threads ──────────────────────────────────────────────────────────────
# Pyodide cannot start Python threads. BackgroundImportThread MUST still
# run (Controller.storage busy-waits on the _storage it seeds) — inline is
# fine, it's a finite pre-import pass. Everything else (toast managers,
# microsd watcher, screensaver helpers) is cosmetic; skip them.
from seedsigner.models import threads as _ss_threads

def _thread_start(self):
    if type(self).__name__ == "BackgroundImportThread":
        self.run()

_ss_threads.BaseThread.start = _thread_start


# ── display driver → JS canvas ───────────────────────────────────────────
from pyodide.ffi import to_js
from PIL import Image

from seedsigner.hardware.displays import display_driver as _dd


class CanvasDisplay(_dd.BaseDisplayDriver):
    """Replaces ST7789/ILI9341 SPI drivers; frames go to the page canvas."""

    def __init__(self, display_type, width, height):
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
