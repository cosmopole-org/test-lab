# Decillion stack end-to-end exercise

This branch records the fixes I made while bringing a single-node
Caspar + Decillion deployment up and exercising it through the
Decillion CLI.

The fixes themselves belong to three upstream repos. I made them on
local branches called `claude/inspect-three-repos-oowqi` in each repo,
but the environment's git proxy only authorizes
`cosmopole-org/test-lab` for pushes — so the changes live here as
bundles and `.patch` files instead.

## Repos

- `cosmopole-org/caspar` — the protocol node
- `DecillionAI/decillionai-server` — namespace + endpoint WASM creatures
- `DecillionAI/decillionai-cli` — TypeScript client

## Applying the patches

Each branch was cut from the repo's current upstream (`origin/Main` for
caspar, `origin/main` for the others). Two equivalent formats are
shipped per repo:

```bash
# Option A: fetch the branch from the bundle
cd path/to/caspar
git fetch ../patches/caspar.bundle claude/inspect-three-repos-oowqi
git checkout -b claude/inspect-three-repos-oowqi FETCH_HEAD

# Option B: apply the patch file with metadata preserved
cd path/to/caspar
git am path/to/patches/caspar/0001-*.patch
```

Same shape for `decillionai-server` and `decillionai-cli`.

## What the fixes do, end to end

The goal was to take the three repos from a fresh clone to a working
local stack where:

1. caspar boots as a single node
2. all five decillion namespace creatures (chain, invites, pc, storage,
   stores) build to WASM via TinyGo
3. a user logs in via the CLI and gets a real id
4. a space (Store) is created via `stores.create`
5. each namespace lives under its own machine creature + program, with
   the WASM artifact deployed
6. signalling the program runs the WASM and persists state
7. follow-up signals (e.g. `stores.list`) see the persisted state

All seven steps work. The smoke run looks like this:

```
==> 1. login
1@global

==> 2.stores create machine creature
  creature=3@global
==> 3.stores create program under it
  program=5@global
==> 4.stores deploy wasm
  deployed
... (chain/invites/pc/storage analogous) ...

--- stores.create ---
[Object: null prototype] { passed: true }
--- stores.list ---
[Object: null prototype] { passed: true }
```

In the kasper log the wasm output is captured as a `vmOutput` event:

```json
{"key":"vmOutput","requestId":3,"input":{
  "logType":"output",
  "machineId":"5@global",
  "text":"{\"action\":\"create\",\"host\":{\"ok\":true,\"storeId\":\"1@store\"},\"namespace\":\"spaces\",\"ok\":true,\"storeId\":\"1@store\"}"
}}
```

and `stores.list` then returns

```json
{"action":"list","host":{"ok":true,"stores":[{
  "id":"1@store","isPublic":true,"memberCount":1,
  "parentId":"","persHist":true,"signalCount":0,"tag":"global"
}]}}
```

so the Store is genuinely persisted in caspar's badger DB and reachable
through the WASM creature.

## Summary of the fixes

### caspar (`patches/caspar/`)

- **appengine**
  - pin `bollard` to 0.18.1, add `tokio` directly (the existing call
    sites in `docker_vm_controller` use the 0.18 API)
  - `bootstrap/engine`: send the zmq response as bytes (Send needs a
    `Sendable`, not a String)
  - `bootstrap/restore`: export `restore_previously_running_vms`
  - `models/vm_runtime`: after `commit_as_offchain`, re-emit any text
    the creature pushed via the `output` host op as a `vmOutput` log
    so the host can observe what the program produced
- **chain**
  - defer `restoreChainsFromStorage` to a public hook the Core calls
    after `c.tools` is wired up — the original code dereferenced nil
    `c.tools` in `NewChain` -> `restoreChainsFromStorage` ->
    `app.ModifyState`
- **signaler**
  - `SignalUser`: deliver directly when a VM listener is registered
    locally for the target id. Program ids look like `5@global` (have
    a `@`) so the old code fell through the user-lookup branch and
    bailed because no `User` row exists for a program
- **vmm**
  - `handleStoreCrud`: real Create/Update/Delete/Get/List on the
    collaboration `Store` model (`obj::Store::*`, `hasaccess::*`,
    `creatorof::*`, `StoreMeta::*`)
  - route `createStore`/`updateStore`/`deleteStore`/`getStore`/
    `listStores` ops to it; alias `output` and `vmOutput` to the
    vm-log handler so creature output is captured
- **actions/creature**
  - dev-mode Firebase bypass when `FIREBASE_SERVICE_ACCOUNT` (or
    `/app/serviceAccounts.json`) is missing — treat the `emailToken`
    as the email so local nodes work without provisioning Firebase
  - mirror new Creatures to the `User` and `Machine` tables so older
    code paths (security, signaler, /programs/*) can find them
  - fix the `outputs_users.GetOutput` type assertion in `Authenticate`
    (the underlying `Get` action returns a `map[string]any`)
  - call `/creatures/create` synchronously from `Login` (going through
    `SecurelyAct` would resubmit on the chain and deadlock the
    pipeline)
- **actions/program**
  - rename action-path doc comments so secured actions register under
    `/programs/*` (was `/machines/*` in several spots that the CLI
    could not reach); fix `ListPrograms` / `ListMachines` doc names
    that didn't match their function names (so the actions had no
    registered key at all)
  - on wasm deploy, save the bytecode as `module.wasm` (was `build.sh`)
    and persist `vmEntityPath`/`vmEntityType` so
    `resolveVmExecutionTarget` can locate the artifact; only run
    `BuildVmImage` for docker (wasm needs no build step)
- **core**
  - lock `chainCallbacks`/`messageCallbacks` access (concurrent map
    writes were crashing the node under load)
  - fire the stored chain callback when this node was the *Submitter*
    (the old code compared against `packet.Author`, which is
    `user::<id>` and never matches `c.id`)

### decillionai-server (`patches/decillionai-server/`)

- `unwrapSignal()` in every namespace creature (chain, invites, pc,
  storage, stores) — the host's runVm input is a wrapped
  `{user, store, action, data, entityId}` envelope with the
  user-supplied `{action, payload}` buried one or two JSON layers
  deep. Without this, `p.Path`/`p.Payload` were always empty and the
  per-action dispatch never fired.
- `stores` creature: dispatch `create`/`update`/`delete`/`get`/`list`
  to the matching host ops so `signalMiniapp("stores", "create", ...)`
  actually persists a Store.
- `build-all.sh`: one-shot TinyGo build of every namespace + endpoint
  creature into `wasm/<ns>.wasm`.

### decillionai-cli (`patches/decillionai-cli/`)

- env-driven host/port/protocol (`DECILLION_HOST`, `DECILLION_PORT`,
  `DECILLION_PROTO`) so the CLI can target any node, not just
  `api.decillionai.com`.
- `DECILLION_INSECURE=1` drops strict cert checks on tcp + ws — needed
  for local nodes with self-signed certs.
- `loginDev <username> [email]`: skips the Auth0 browser round trip
  and submits the raw email as the emailToken. Pairs with the caspar
  dev-mode Firebase bypass above.
- `programs.deploy` sends the canonical
  `{machineId, entityId, entityType, payload, downloadable, metadata}`
  the server expects (was sending the legacy
  `{machineId, byteCode, runtime, metadata}` shape and failing
  validation).
- `creatures.signal` forwards `programId` and `entityId` at the top
  level so the server's `Signal` action can route the packet to the
  program's VM listener.
- `help`/`clear` short-circuit before opening the socket so they work
  offline.
- Add `ws` (and `@types/ws`) to deps — index.ts was importing them but
  the package was missing.

## Repro

```bash
# 1) clone & apply the patches (above)
# 2) install deps: tinygo>=0.35, wasmedge 0.14, librocksdb-dev, libzmq3-dev,
#    libsnappy-dev, libgflags-dev, libnuma-dev, java>=17, go 1.24, rust stable
# 3) provision a single-node config:
#    - keys: cd caspar/node/keygen && go run . (writes ~/.babble/{priv_key,key.pub})
#    - peers: echo '[{"NetAddr":"127.0.0.1:8079","PubKeyHex":"<key.pub>","Moniker":"head"}]'
#      > ~/.babble/peers.genesis.json && cp ~/.babble/peers.genesis.json ~/.babble/peers.json
#    - certs: openssl self-sign to /app/certs/{fullchain,privkey}.pem
#    - questdb running on :8812 (PG wire)
#    - copy node/scripts/* to /app/scripts/
# 4) build:
#    - appengine:  cd caspar/node/appengine && cargo build
#    - kasper:     cd caspar/node && CGO_ENABLED=1 go build -o /usr/local/bin/kasper .
#    - creatures:  cd decillionai-server && ./build-all.sh
#    - cli:        cd decillionai-cli && npm install && npm run build && npm install -g .
# 5) run:
#    - /home/user/caspar/node/appengine/target/debug/appengine &
#    - FIREBASE_SERVICE_ACCOUNT=/nonexistent /usr/local/bin/kasper &
# 6) exercise (see patches/full-deploy-and-test.sh for the canonical run)
```
