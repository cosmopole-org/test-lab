# Getting Started 🚀

> Updated: **2026-04-14**

This guide matches the current repo layout (`node/`, `cmd/casparctl/`, `sdk/`).

## 1) Prerequisites

- Go `1.24.x`
- Rust + Cargo (for `node/appengine`)
- Docker (recommended path uses `casparctl install`)
- TLS cert/key files
- Firebase service account JSON
- QuestDB (for storage/log flows using PG wire)

## 2) Environment Setup

```bash
cd node
cp sample.env .env
```

Important variables include:

- `OWNER_ID`, `OWNER_PRIVATE_KEY`, `ORIGIN`
- `CLIENT_WS_API_PORT`, `CLIENT_TCP_API_PORT`, `FEDERATION_API_PORT`
- `BLOCKCHAIN_API_PORT`, `ENTITY_API_PORT`, `VM_API_PORT`
- `TELEMETRY_API_PORT` (default `9099`), `TELEMETRY_DB_PATH`
- `STORAGE_ROOT_PATH`, `BASE_DB_PATH`, `APPLET_DB_PATH`

## 3) Build

```bash
cd node/appengine
cargo build

cd ../
CGO_ENABLED=1 go build -o kasper .
```

## 4) Run Options

### A) CLI-managed container flow (recommended) ✅

```bash
cd cmd/casparctl
go install .

cd ../../node
casparctl install --name caspar-node
casparctl stats
```

Useful controls:

```bash
casparctl start
casparctl pause
casparctl resume
casparctl stop
casparctl uninstall
casparctl purge
```

### B) Direct binary flow

```bash
cd node
./kasper
```

## 5) Smoke Checks 🧪

- Node starts without panic
- Telemetry endpoint returns snapshot: `GET /telemetry/snapshot`
- Chain service endpoints answer (`/stats`, `/peers`)
- Auth key route works (`/auths/getServerPublicKey`)

## 6) Recent Operational Notes 📌

- VM build/runtime logs are now persisted and streamable.
- Wasm build-image support was extended in recent updates.
- Appengine host-op naming now uses `run_db_op` (renamed from docker-specific naming).
