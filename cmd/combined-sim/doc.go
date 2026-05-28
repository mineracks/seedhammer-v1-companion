// Command combined-sim wires composer + emulator + SeedSigner-sim into a
// single browser page with a QR-handoff bus.
//
// The handoff bus has two modes:
//
//   - Display mode (faithful): a sim renders a QR on its canvas; the
//     SeedHammer emulator's mock camera reads pixels from that canvas
//     and runs them through the upstream QR decoder. Same code path as
//     a real device.
//
//   - Direct mode (fast): a shared JS bus copies the payload between sims
//     directly, skipping QR encode/decode. Useful for debugging.
//
// Status: STUB — depends on cmd/composer, cmd/emulator, cmd/seedsigner-sim
// landing first.
//
// Modelled on Gangleri42's cmd/webnfc-sim:
// https://github.com/Gangleri42/seedhammer/tree/seedhammer-features/cmd/webnfc-sim
package main
