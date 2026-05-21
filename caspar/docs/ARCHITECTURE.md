# Architecture 🧠

> Updated: **2026-04-14**

## 1) Runtime Topology

```text
TLS TCP/WS Clients
  -> Action Router + Guards
    -> Core Transactions
      -> Hashgraph Chain
      -> Federation Bridge
      -> VM Runtime Drivers
      -> Storage + Entity/Stream APIs
```

## 2) Core Subsystems

- **Core module** (`node/src/core/module/core`): action dispatch, transaction lifecycle, callbacks, updates
- **Action/pluggers** (`node/src/shell/api/actions`, `node/src/shell/api/pluggers`): route wiring and command handlers
- **Appengine runtime** (`node/appengine/src`): VM controllers, host functions, runtime orchestration
- **Network drivers** (`node/src/drivers/network`): chain + federation + clients
- **Storage drivers**: Badger + QuestDB/PG-wire patterns

## 3) Recent Architectural Changes 🆕

- Runtime/build log capture and streaming were expanded across VM runtimes.
- Host db operation naming was generalized to `run_db_op`.
- Docker entity build/run behavior was aligned around `entityId`.
- Runtime context hierarchy now uses universal cache paths.

## 4) Security Model 🔒

- Signed request packets (identity + signature validation)
- Guard-based authorization on action routes
- Store/member/access checks before state mutation
- Federated callbacks validated against known origins

## 5) Startup Composition

Startup wires adapters/tools, installs actions/pluggers, and then runs:

- pprof (`:9999`)
- client TLS servers (`ws` + `tcp`)
- federation transport
- chain service
- entity/VM stream APIs
- telemetry API (`/telemetry/snapshot`)
