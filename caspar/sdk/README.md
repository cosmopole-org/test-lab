# SDK Runtime Samples 🧪

> Updated: **2026-04-14**

This folder contains runtime-oriented examples for Caspar VM execution environments.

## Included samples

- `wasm/` — host-call compatible module pattern (`malloc`, `run`, `env.hostCall`)
- `docker/` — packet + callback-compatible runtime sample
- `javascript/` — QuickJS bridge pattern with host callbacks
- `elpify/` — MASM + elpify example modules
- `elpian/` — AST-style logic sample
- `creatures/` — namespace-oriented WASM actions moved from legacy shell-path handling

## Recent Notes 🆕

- Creatures were moved under `sdk/creatures` and are now the primary VM endpoint modules.
- Keep examples additive so integrators can diff safely between revisions.
