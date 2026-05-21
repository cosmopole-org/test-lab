#!/bin/bash
# End-to-end exercise of the Decillion stack on a single-node Caspar:
#   - login (dev mode)
#   - one machine creature per namespace
#   - one program per machine, with the corresponding wasm deployed
#   - signal each program and capture its response via the new async
#     creature-signal/result reply channel
set -e
cd "$(dirname "$0")"
ROOT="$(pwd)"
export DECILLION_HOST=127.0.0.1
export DECILLION_PORT=8076
export DECILLION_PROTO=ws
export DECILLION_INSECURE=1

WORKDIR=/tmp/decillion-cli-workdir
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"
cd "$WORKDIR"

USERNAME="${DECILLION_USERNAME:-claude}"
EMAIL="${DECILLION_EMAIL:-claude@dev.local}"

echo "==> 1. login"
decillion --batch "loginDev $USERNAME $EMAIL" >/dev/null
echo "  userId=$(cat auth/userId.txt)"

declare -A WASMS=(
  [stores]=$ROOT/decillionai-server/wasm/stores.wasm
  [invites]=$ROOT/decillionai-server/wasm/invites.wasm
  [storage]=$ROOT/decillionai-server/wasm/storage.wasm
  [chain]=$ROOT/decillionai-server/wasm/chain.wasm
  [pc]=$ROOT/decillionai-server/wasm/pc.wasm
)
declare -A NS_TO_KEY=( [stores]=STORES [invites]=INVITES [storage]=STORAGE [chain]=CHAINS [pc]=PC )
declare -A CREATURE_ID
declare -A PROGRAM_ID

extract_id() { grep -oE "id: '[0-9]+@global'" | head -1 | sed -E "s/^id: '([^']+)'$/\1/"; }

for ns in "${!WASMS[@]}"; do
  wasm="${WASMS[$ns]}"
  pkg="/tmp/${ns}-pkg"
  rm -rf "$pkg" && mkdir -p "$pkg/builder" "$pkg/src"
  cat > "$pkg/builder/build.sh" <<EOF
#!/bin/bash
cp $wasm "\$(dirname "\$0")/bytecode"
EOF
  chmod +x "$pkg/builder/build.sh"
  cp "$ROOT/decillionai-server/creatures/$ns/main.go" "$pkg/src/"
  cp "$ROOT/decillionai-server/creatures/$ns/go.mod" "$pkg/src/"

  echo "==> 2.$ns create machine creature"
  out=$(decillion --batch "creatures.createMachine main ${ns}-app ${ns}-app ${ns}-namespace" 2>&1)
  CREATURE_ID[$ns]=$(echo "$out" | extract_id)
  echo "  creature=${CREATURE_ID[$ns]}"

  echo "==> 3.$ns create program"
  out=$(decillion --batch "programs.create ${ns}-prog ${CREATURE_ID[$ns]} /$ns wasm ${ns}-program" 2>&1)
  PROGRAM_ID[$ns]=$(echo "$out" | extract_id)
  echo "  program=${PROGRAM_ID[$ns]}"

  echo "==> 4.$ns deploy wasm"
  decillion --batch "programs.deploy ${PROGRAM_ID[$ns]} $pkg wasm {}" >/dev/null
  echo "  deployed"
done

for ns in "${!WASMS[@]}"; do
  key="${NS_TO_KEY[$ns]}"
  export DECILLION_${key}_CREATURE_ID="${CREATURE_ID[$ns]}"
  export DECILLION_${key}_PROGRAM_ID="${PROGRAM_ID[$ns]}"
  export DECILLION_${key}_ENTITY="main"
done

echo
echo "==> 5. exercise stores via creatures.signal (async req/res)"
echo
echo "--- stores.create ---"
decillion --batch "stores.create true true global my-first-space" 2>&1 | tail -10
echo
echo "--- stores.list ---"
decillion --batch "stores.list 0 10" 2>&1 | tail -15

echo
{ for ns in "${!WASMS[@]}"; do
    key="${NS_TO_KEY[$ns]}"
    echo "export DECILLION_${key}_CREATURE_ID=${CREATURE_ID[$ns]} DECILLION_${key}_PROGRAM_ID=${PROGRAM_ID[$ns]} DECILLION_${key}_ENTITY=main"
  done
} > /tmp/decillion-deploy-env.sh
chmod +x /tmp/decillion-deploy-env.sh
echo "env exports written to /tmp/decillion-deploy-env.sh"
