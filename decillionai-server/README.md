# Decillion Platform Server 🌐🧬⚙️

This repository contains the **Decillion platform server-side creature workspace** used to power programmable operations in the Decillion ecosystem.

At a high level, the platform can be understood as:

- **`decillionai-app`**: the user-facing application layer (frontend/product experience).
- **`decillionai-server` (this repo)**: the server-side creature runtime modules and endpoint-oriented WASM programs.
- **Caspar-compatible host runtime**: the execution substrate where creature logic is deployed and invoked.

---

## What this repository is for 📦

The code in this repository focuses on **Go-based WASM creature modules** that:

- accept packetized input (`path`, `payload`, context identifiers),
- route behavior by namespace and endpoint,
- invoke host operations through a wasm-imported `hostCall`,
- persist activity/state snapshots through host DB/JSON ops,
- return normalized output packets back to the platform runtime.

In practical terms, this repo is where platform capabilities are represented as deployable **creature programs** and **endpoint modules**.

---

## Core architecture (server side) 🏗️

### 1) Namespace creatures

Under `creatures/`, each domain namespace is an independent Go module:

- `chain`
- `invites`
- `pc`
- `storage`
- `stores`

Each namespace module follows a common runtime pattern:

1. Decode host input packet.
2. Resolve creature/program/entity context.
3. Execute host operations (`putJson`, `dbOp`, `output`, etc.).
4. Emit a JSON response for runtime consumers.

### 2) Endpoint creatures

Under `creatures/endpoints/`, each endpoint is separated into its own module. Domains currently include:

- `chains/*`
- `invites/*`
- `pc/*`
- `storage/*`
- `stores/*`

This modular structure makes endpoint deployment and versioning granular and safer for iterative releases.

### 3) Verifiable execution extension

`creatures/verifiable-chain/` adds a container/VM-oriented verification layer that supports:

- executor/verifier node roles,
- proof-sharing and vote workflows,
- shard and validator-related chain utilities.

---

## Relationship to `decillionai-app` 🔗

The broader Decillion platform typically separates concerns as follows:

- **App repo (`decillionai-app`)** handles user flows, orchestration UX, and application-facing interactions.
- **Server repo (`decillionai-server`)** provides the runtime-executable creature logic behind those flows.

So when a product feature is triggered in the app, the corresponding server-side effect is commonly implemented by creature namespaces/endpoints from this repository.

---

## Runtime model in brief ⚡

Each creature module is compiled to WASM and executed by a host that exposes an imported function:

- `//go:wasmimport env hostCall`

This enables creature logic to request host-managed capabilities such as:

- data writes/reads,
- state snapshots,
- output emission,
- runtime context handling.

This model keeps business logic portable while delegating privileged operations to the host runtime.

---

## Repository structure overview 🗂️

```text
.
├─ creatures/
│  ├─ chain/
│  ├─ invites/
│  ├─ pc/
│  ├─ storage/
│  ├─ stores/
│  ├─ endpoints/
│  │  ├─ chains/*
│  │  ├─ invites/*
│  │  ├─ pc/*
│  │  ├─ storage/*
│  │  └─ stores/*
│  └─ verifiable-chain/
└─ README.md
```

---

## Typical developer workflow 🛠️

1. Implement or update a namespace/endpoint creature.
2. Build the creature artifact (WASM/module output depending on target flow).
3. Deploy using Decillion tooling/CLI workflows.
4. Trigger endpoint paths from the app/runtime.
5. Observe output packets and persisted state in the host environment.

---

## Design goals ✅

- **Modularity:** one module per namespace/endpoint for clean ownership.
- **Deterministic host bridge:** all privileged actions flow through host ops.
- **Composable platform growth:** add new capabilities by adding creature endpoints.
- **Verifiability-ready path:** optional extension for execution proof workflows.

---

## Notes for contributors 🧭

- Keep naming aligned with runtime route conventions.
- Preserve module boundaries (`go.mod` per creature module).
- Keep packet/input contracts stable when updating endpoint logic.
- Treat verifiable-chain examples as extensible scaffolding for production-hardening.

---

## Ecosystem pointers 🌌

- **App layer:** `DecillionAI/decillionai-app`
- **CLI/tooling:** `DecillionAI/decillionai-cli`
- **Protocol/runtime reference:** `cosmopole-org/caspar`

If you're onboarding to the Decillion platform, start from the app flows, then map each feature to the corresponding server creature namespace/endpoint in this repository.
