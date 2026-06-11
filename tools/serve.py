#!/usr/bin/env python3
"""Dev server for web/ with cross-origin-isolation headers.

The SeedSigner sim needs SharedArrayBuffer (worker input + Atomics sleeps),
which browsers only enable when the page is cross-origin isolated. Plain
`python3 -m http.server` can't add the COOP/COEP headers, so use this instead:

    python3 tools/serve.py [port]      # default 38080

Then open  http://localhost:<port>/seedsigner-sim/  (or /composer/, /emulator/
— the extra headers are harmless for the Go-WASM shells).

Note: COEP require-corp means every cross-origin subresource must opt in via
CORS/CORP. The Pyodide CDN (jsdelivr) does. If we later vendor Pyodide for
offline use, nothing here changes.
"""
import http.server
import os
import sys


class Handler(http.server.SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header("Cross-Origin-Opener-Policy", "same-origin")
        self.send_header("Cross-Origin-Embedder-Policy", "require-corp")
        self.send_header("Cross-Origin-Resource-Policy", "cross-origin")
        self.send_header("Cache-Control", "no-store")
        super().end_headers()


Handler.extensions_map.update({
    ".wasm": "application/wasm",
    ".mjs": "text/javascript",
})


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 38080
    web_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "web")
    os.chdir(web_dir)
    server = http.server.ThreadingHTTPServer(("", port), Handler)
    print(f"serving {os.path.abspath(web_dir)} on http://localhost:{port}/ "
          "(COOP/COEP enabled)")
    server.serve_forever()


if __name__ == "__main__":
    main()
