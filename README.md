# Decillion stack end-to-end exercise

This branch is a single working tree of three upstream repos plus the
fixes I made while bringing them up as a single-node deployment and
exercising them through the Decillion CLI.

## Layout

- `caspar/` — protocol node (Go) + appengine (Rust) + `cmd/casparctl`
  + `sdk/`. Upstream: `cosmopole-org/caspar`, branched from `Main`.
- `decillionai-server/` — namespace + endpoint WASM creatures (Go,
  built with TinyGo). Upstream: `DecillionAI/decillionai-server`,
  branched from `main`.
- `decillionai-cli/` — TypeScript client. Upstream:
  `DecillionAI/decillionai-cli`, branched from `main`.
- `full-deploy-and-test.sh` — end-to-end script that logs in, creates
  a machine creature + program per namespace, deploys each wasm, then
  runs `stores.create` and `stores.list`.

## What works

A fresh clone of each upstream repo would not even build the appengine
crate, and even after building, every WASM creature would receive an
empty `path`/`payload` from the framework and short-circuit. After the
fixes in this branch the full happy path runs:

1. `caspar` boots as a single node (chain + WS/TCP/federation listeners).
2. All five decillion namespace creatures (chain, invites, pc, storage,
   stores) build to WASM via TinyGo.
3. A user logs in via the CLI and gets a real id (`1@global`).
4. A space (Store) is created via `stores.create`.
5. Each namespace lives under its own machine creature + program, with
   the WASM artifact deployed.
6. Signalling the program runs the WASM and persists state.
7. Follow-up signals (e.g. `stores.list`) see the persisted state.

In the kasper log the wasm's response is captured as a `vmOutput` event:

```json
{"key":"vmOutput","requestId":3,"input":{
  "logType":"output",
  "machineId":"5@global",
  "text":"{\"action\":\"create\",\"host\":{\"ok\":true,\"storeId\":\"1@store\"},\"namespace\":\"spaces\",\"ok\":true,\"storeId\":\"1@store\"}"
}}
```

and `stores.list` then returns:

```json
{"action":"list","host":{"ok":true,"stores":[{
  "id":"1@store","isPublic":true,"memberCount":1,
  "parentId":"","persHist":true,"signalCount":0,"tag":"global"
}]}}
```

so the Store is genuinely persisted in caspar's badger DB and reachable
through the WASM creature on read-back.

## Summary of the fixes

### `caspar/`

- **appengine** (`caspar/node/appengine/`)
  - `Cargo.toml`: pin `bollard = 0.18.1` (matches the call sites in
    `docker_vm_controller`); add `tokio` as a direct dependency
  - `src/bootstrap/engine.rs`: send the zmq response as bytes (Send
    needs a `Sendable`, not a `String`)
  - `src/bootstrap/restore.rs`: export `restore_previously_running_vms`
    so `mod.rs` can re-export it
  - `src/models/vm_runtime.rs`: after `commit_as_offchain`, re-emit any
    text the creature pushed via the `output` host op as a `vmOutput`
    log so the host platform can observe what the program produced
- **chain** (`caspar/node/src/drivers/network/chain/chain.go`)
  - defer `restoreChainsFromStorage` to a new `RestoreFromStorage()`
    hook that `Core.Load` calls after `c.tools` is wired up. Before
    this fix, `NewChain` -> `restoreChainsFromStorage` ->
    `app.ModifyState` ran during driver construction and dereferenced
    nil `c.tools`.
- **signaler** (`caspar/node/src/drivers/signaler/signaler.go`)
  - `SignalUser` now delivers directly when a VM listener is registered
    locally for the target id. Program ids look like `5@global` (contain
    a `@`) so the old code took the user-lookup branch and bailed
    because no `User` row exists for a program; the VM listener
    registered by `Vmm.Assign(programId)` was never reached.
- **vmm** (`caspar/node/src/drivers/vmm/`)
  - `hostcall_entities.go`: implement `handleStoreCrud` for the
    collaboration `Store` model — real Create/Update/Delete/Get/List on
    `obj::Store::*`, `hasaccess::*`, `creatorof::*`, `StoreMeta::*`.
  - `hostcall_global.go`: route
    `createStore`/`updateStore`/`deleteStore`/`getStore`/`listStores`
    to the new handler; alias `output` and `vmOutput` to the vm-log
    handler so creature output is captured.
- **actions/creature** (`caspar/node/src/shell/api/actions/creature/creature.go`)
  - dev-mode Firebase bypass when `FIREBASE_SERVICE_ACCOUNT` (or
    `/app/serviceAccounts.json`) is missing — treat the `emailToken`
    as the email so local nodes work without provisioning Firebase Auth.
  - mirror newly created Creatures to the `User` and `Machine` tables
    so older code paths (security, signaler, `/programs/*`) can find
    them by id.
  - fix the `outputs_users.GetOutput` type assertion in `Authenticate`
    (the underlying `/creatures/get` action returns
    `map[string]any{"creature": Creature}`, not `GetOutput`).
  - call `/creatures/create` synchronously from `Login` (going through
    `SecurelyAct` would resubmit on the chain and deadlock the chain
    pipeline, which is single-threaded).
- **actions/program** (`caspar/node/src/shell/api/actions/program/program.go`)
  - rename action-path doc comments so secured actions register under
    `/programs/*` (was `/machines/*` in several spots that the CLI
    could not reach); fix the `ListPrograms`/`ListMachines` doc names
    that didn't match their function names (so those actions had no
    registered key at all — the doc-extractor couldn't find them).
  - on wasm deploy, save the bytecode as `module.wasm` (was
    `build.sh`) and persist `vmEntityPath`/`vmEntityType` links so
    `resolveVmExecutionTarget` can locate the artifact; only run
    `BuildVmImage` for docker (wasm needs no build step).
- **core** (`caspar/node/src/core/module/core/core.go`)
  - lock `chainCallbacks`/`messageCallbacks` access (concurrent map
    writes were crashing the node under load — `fatal error:
    concurrent map writes`).
  - fire the stored chain callback when this node was the `Submitter`
    (the old code compared against `packet.Author`, which is
    `user::<id>` and never matches `c.id`).
- **abstract** (`caspar/node/src/abstract/adapters/network/chain.go`)
  - add `RestoreFromStorage()` to `IChain` for the new deferred restore.

### `decillionai-server/`

- `unwrapSignal()` added to every namespace creature
  (`creatures/{chain,invites,pc,storage,stores}/main.go`) — the host's
  runVm input is a wrapped `{user, store, action, data, entityId}`
  envelope with the user-supplied `{action, payload}` buried one or two
  JSON layers deep. Without this, `p.Path`/`p.Payload` were always
  empty and the per-action dispatch never fired.
- The `stores` creature dispatches `create`/`update`/`delete`/`get`/`list`
  to the matching host ops, so `signalMiniapp("stores", "create", ...)`
  actually persists a `Store`.
- `build-all.sh`: one-shot TinyGo build of every namespace + endpoint
  creature into `wasm/<ns>.wasm`.

### `decillionai-cli/`

- env-driven host/port/protocol (`DECILLION_HOST`, `DECILLION_PORT`,
  `DECILLION_PROTO`) so the CLI can target any node, not just
  `api.decillionai.com`.
- `DECILLION_INSECURE=1` drops strict cert checks on tcp + ws — needed
  for local nodes with self-signed certs.
- `loginDev <username> [email]`: skips the Auth0 browser round trip
  and submits the raw email as the `emailToken`. Pairs with the caspar
  dev-mode Firebase bypass above.
- `programs.deploy` sends the canonical
  `{machineId, entityId, entityType, payload, downloadable, metadata}`
  shape the server's `DeployInput` expects (was sending the legacy
  `{machineId, byteCode, runtime, metadata}` shape and failing
  validation).
- `creatures.signal` forwards `programId` and `entityId` at the top
  level so the server's `Signal` action can route the packet to the
  program's VM listener (not just the parent creature).
- `help`/`clear` short-circuit before opening the socket so they work
  offline.
- Add `ws` (and `@types/ws`) to dependencies — `index.ts` was importing
  them but the package was missing from `package.json`.

## Repro

Prerequisites: `tinygo >= 0.35` (we used 0.39), WasmEdge 0.14,
`librocksdb-dev`, `libzmq3-dev`, `libsnappy-dev`, `libgflags-dev`,
`libnuma-dev`, `default-jre-headless` (>=17), `go 1.24`, `rust stable`,
QuestDB.

```bash
# 1) Provision a single-node config
#    keys
cd caspar/node/keygen && go run .         # writes ~/.babble/{priv_key,key.pub}
PUB=$(cat ~/.babble/key.pub)
cat > ~/.babble/peers.genesis.json <<EOF
[{"NetAddr":"127.0.0.1:8079","PubKeyHex":"$PUB","Moniker":"head"}]
EOF
cp ~/.babble/peers.genesis.json ~/.babble/peers.json

#    certs (self-signed)
mkdir -p /app/certs && openssl req -x509 -newkey rsa:2048 \
  -keyout /app/certs/privkey.pem -out /app/certs/fullchain.pem \
  -sha256 -days 3650 -nodes -subj "/CN=localhost" \
  -addext "subjectAltName = DNS:localhost,IP:127.0.0.1"

#    QuestDB (PG wire on :8812)
bash caspar/node/scripts/install-questdb.sh
/app/questdb/questdb.sh start -d /app/questdb

#    runtime scripts caspar expects under /app
mkdir -p /app/scripts && cp caspar/node/scripts/*.sh /app/scripts/

# 2) Build
cd caspar/node/appengine && cargo build
cd ../ && CGO_ENABLED=1 go build -o /usr/local/bin/kasper .
cd ../../decillionai-server && ./build-all.sh
cd ../decillionai-cli && npm install && npm run build && npm install -g .

# 3) Write the .env for caspar (sample.env documents all keys)
#    set OWNER_ID, OWNER_PRIVATE_KEY (PKCS8 PEM, escaped \n), the API
#    ports (8076 ws / 8077 tcp / 8078 fed / 8079 chain / 9099 telemetry),
#    STORAGE_ROOT_PATH=/app/storage, BASE_DB_PATH=/app/db etc., and
#    IS_HEAD=true.

# 4) Run
cd caspar/node/appengine && ./target/debug/appengine &
cd caspar/node && FIREBASE_SERVICE_ACCOUNT=/nonexistent /usr/local/bin/kasper &

# 5) Exercise
bash full-deploy-and-test.sh
```
