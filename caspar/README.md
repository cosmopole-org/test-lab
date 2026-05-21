# Caspar Protocol 🌐

> **Status:** Documentation updated on **2026-04-14**.

Caspar is a decentralized protocol stack with hashgraph-style consensus, federation-native messaging, and multi-runtime execution (`wasm`, `docker`, `javascript`, `elpify`, `elpian`, `firecracker`).

## ✨ Highlights

- Deterministic action ordering through chain + core transaction flow
- Federation bridge for cross-origin requests and updates
- Signed packet protocol over TLS (`tcp` + `ws`)
- Runtime host operations for VM build/run/signal/storage workflows
- Telemetry snapshot API with CLI dashboard integration

## 🧭 Repository Map

- `node/` — main node runtime (Go + Rust appengine)
- `cmd/casparctl/` — operator CLI for install/control/dashboard
- `sdk/` — runtime examples + VM creatures
- `docs/` — architecture, API, setup guides
- `sdk/creatures/verifiable-chain/` — optional chain extension toolkit

## 🕘 Recent Changes History

- **2026-04-14**
  - Reorganized project structure and runtime files for clearer boundaries.
  - Persisted and streamed build/runtime logs across VM runtimes.
  - Renamed host function `run_docker_db_op` → `run_db_op` in appengine host paths.
- **2026-04-13**
  - Extended build-image API behavior to support wasm-oriented flows.
  - Unified docker entity build/run packet behavior around `entityId`.
- **2026-04-11**
  - Added `casparctl` lifecycle CLI + live telemetry dashboard.
  - Added telemetry HTTP server and cached snapshot support.
  - Updated VM host hierarchy resolution to use a universal runtime context cache.
- **2026-04-10**
  - Moved `creatures` into `sdk/creatures`.
  - Added verifiable-chain scaffolding with PoS election + shard planning (now in `sdk/creatures/`).
  - Refreshed markdown docs and route naming notes.

## 🛠️ `casparctl` CLI

```bash
cd cmd/casparctl
go install .

casparctl install --name caspar-node
casparctl stats
casparctl pause
casparctl resume
casparctl stop
casparctl uninstall
casparctl purge
```

`casparctl stats` reads telemetry snapshots from `TELEMETRY_API_PORT` (default `9099`) and renders a live TUI.

## 📚 Documentation

- `docs/GETTING_STARTED.md`
- `docs/API_REFERENCE.md`
- `docs/ARCHITECTURE.md`
- `sdk/README.md`
- `sdk/creatures/README.md`
- `sdk/creatures/verifiable-chain/README.md`
