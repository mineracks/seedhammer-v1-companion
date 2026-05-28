# Documentation

## architecture/

Authoritative design docs for the project. These are the contract — every
implementation decision should be traceable back to one of these.

| File | What it answers |
|---|---|
| [BASELINES.md](architecture/BASELINES.md) | Which exact upstream SHAs this project is built on top of, license findings, path-mapping table for the v1.0.0 → v1.3.0 layout refactor. |
| [v1-engrave-spec.md](architecture/v1-engrave-spec.md) | The MarkingWay USB-serial wire protocol used by upstream v1 — opcodes, framing, plate geometry, font format. |
| [v1-buttons-and-ui.md](architecture/v1-buttons-and-ui.md) | v1's physical button layout, GPIO mapping, screen-flow conventions, proposed emulator keyboard map. |
| [sh1e-spec.md](architecture/sh1e-spec.md) | The new on-the-wire envelope format for shipping plate designs from composer to controller (CBOR + CRC32, deterministic encoding, security analysis). |
| [seedsigner-reuse.md](architecture/seedsigner-reuse.md) | Strategy for bundling a faithful in-browser SeedSigner emulator via Pyodide + supporting both Classic and SeedSigner+ "jumbo" device profiles. |

All five docs were drafted as prep before this repo existed — they originate
in `mineracks-infrastructure/roadmap/seedhammer-v1-companion/prep/`. The
copies here are the live authoritative versions; the prep docs are now
historical.
