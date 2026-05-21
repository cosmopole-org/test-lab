# Creatures SDK (WASM Go Projects) 🧬

> Updated: **2026-04-16**

This folder contains the imported `sdk/creatures` workspace used to build WebAssembly (Go) creature binaries and endpoint-specific wasm modules that execute host operations.

## What is included

### Core namespaces

Each namespace has its own Go module and entrypoint:

- `chain`
- `invites`
- `pc`
- `storage`
- `stores`

### Endpoint modules

The `endpoints/` tree provides route-scoped modules grouped by domain:

- `endpoints/chains`
  - `create`
  - `createFromStore`
  - `createShard`
  - `registerNode`
  - `submitBaseTrx`
- `endpoints/invites`
  - `accept`
  - `cancel`
  - `create`
  - `decline`
  - `listStoreInvites`
  - `listUserInvites`
- `endpoints/pc`
  - `execCommand`
  - `runPc`
- `endpoints/storage`
  - `deleteStoreEntity`
  - `deleteUserEntity`
  - `download`
  - `upload`
  - `uploadAppEntity`
  - `uploadStoreEntity`
  - `uploadUserEntity`
- `endpoints/stores`
  - `addMachine`
  - `addMember`
  - `addProgram`
  - `create`
  - `delete`
  - `get`
  - `getDefaultAccess`
  - `history`
  - `join`
  - `leave`
  - `list`
  - `listStoreMachines`
  - `meta`
  - `read`
  - `readMembers`
  - `removeMachine`
  - `removeMember`
  - `removeProgram`
  - `signal`
  - `update`
  - `updateMember`
  - `updateMemberAccess`
  - `updateProgram`
  - `updateProgramAccess`

### Extension module

- `verifiable-chain/`
  - standalone module with its own `main.go`, `go.mod`, and `Dockerfile`.

## Runtime model

Typical creature execution flow:

1. Read input payload (`path`, `payload`, `userId`, `storeId`, etc.).
2. Resolve route and context values.
3. Invoke host operations through wasm imports.
4. Return host response packet.


## Using DecillionAI CLI

Use [DecillionAI CLI](https://github.com/DecillionAI/decillionai-cli) to scaffold creature programs and deploy creature entities to the Caspar protocol network ([cosmopole-org/caspar](https://github.com/cosmopole-org/caspar)).

### Typical workflow

1. Install and configure DecillionAI CLI (see the CLI repository README for the latest commands and auth setup).
2. Create a new creature program/module with the CLI scaffolding flow.
3. Build and prepare your creature artifacts (WASM/module files) using the generated project setup.
4. Deploy creature entities to a Caspar-connected environment using CLI deployment commands.
5. Verify entity state and endpoint behavior in your target network/workspace.

### What the CLI is used for in this SDK context

- **Program creation:** quickly bootstrap creature projects aligned with this SDK module layout.
- **Entity deployment:** publish and manage creature entities on Caspar protocol environments.
- **Operational workflows:** streamline repeated build/deploy cycles when iterating on endpoint logic.

## Notes for maintainers

- This directory is intentionally organized as many independent Go modules (`go.mod` per creature).
- Keep namespace and endpoint naming aligned with host runtime route names.
- When syncing from upstream sources, preserve folder names exactly to avoid route mismatch.
