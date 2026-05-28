// Command seedsigner-sim is a tiny Go helper that builds the SeedSigner
// emulator's static web assets.
//
// The actual emulation is NOT done in Go — it's done by hosting the
// upstream SeedSigner Python codebase in the browser via Pyodide. See
// docs/architecture/seedsigner-reuse.md for the architecture.
//
// This Go binary's job:
//   - Download a pinned Pyodide release tarball
//   - Pull the upstream SeedSigner Python source at a pinned commit
//   - Verify checksums
//   - Lay them out under web/seedsigner-sim/dist/ for the static-site build
//
// Status: STUB — implementation pending. Pinned commit choices documented
// in web/seedsigner-sim/UPSTREAM.md once that file exists.
package main
