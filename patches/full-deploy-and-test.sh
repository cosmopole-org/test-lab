#!/bin/bash
# End-to-end exercise of the Decillion stack on a single-node Caspar:
#   - login (dev mode)
#   - one machine creature per namespace
#   - one program per machine, with the corresponding wasm deployed
#   - signal each program and capture its output

set -e
cd "$(dirname "$0")"
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
cat auth/userId.txt
echo

# Map: namespace -> wasm artifact path
declare -A WASMS=(
  [stores]=/home/user/decillionai-server/wasm/stores.wasm
  [invites]=/home/user/decillionai-server/wasm/invites.wasm
  [storage]=/home/user/decillionai-server/wasm/storage.wasm
  [chain]=/home/user/decillionai-server/wasm/chain.wasm
  [pc]=/home/user/decillionai-server/wasm/pc.wasm
)
declare -A NS_TO_KEY=( [stores]=STORES [invites]=INVITES [storage]=STORAGE [chain]=CHAINS [pc]=PC )

declare -A CREATURE_ID
declare -A PROGRAM_ID

extract_id() {
  # Pulls "X@global" out of the last JS object decillion prints.
  grep -oE "id: '[0-9]+@global'" | head -1 | sed -E "s/^id: '([^']+)'$/\1/"
}

for ns in "${!WASMS[@]}"; do
  wasm="${WASMS[$ns]}"
  pkg="/tmp/${ns}-pkg"
  rm -rf "$pkg" && mkdir -p "$pkg/builder" "$pkg/src"
  cat > "$pkg/builder/build.sh" <<EOF
#!/bin/bash
cp $wasm "\$(dirname "\$0")/bytecode"
EOF
  chmod +x "$pkg/builder/build.sh"
  cp /home/user/decillionai-server/creatures/$ns/main.go "$pkg/src/"
  cp /home/user/decillionai-server/creatures/$ns/go.mod "$pkg/src/"

  echo "==> 2.$ns create machine creature"
  out=$(decillion --batch "creatures.createMachine main ${ns}-app ${ns}-app ${ns}-namespace" 2>&1)
  CREATURE_ID[$ns]=$(echo "$out" | extract_id)
  echo "  creature=${CREATURE_ID[$ns]}"

  echo "==> 3.$ns create program under it"
  out=$(decillion --batch "programs.create ${ns}-prog ${CREATURE_ID[$ns]} /$ns wasm ${ns}-program" 2>&1)
  PROGRAM_ID[$ns]=$(echo "$out" | extract_id)
  echo "  program=${PROGRAM_ID[$ns]}"

  echo "==> 4.$ns deploy wasm"
  decillion --batch "programs.deploy ${PROGRAM_ID[$ns]} $pkg wasm {}" >/dev/null
  echo "  deployed"
done

# Print env vars needed for the signaling round
echo
echo "==> 5. exercise each namespace creature via creatures.signal"
for ns in "${!WASMS[@]}"; do
  key="${NS_TO_KEY[$ns]}"
  export DECILLION_${key}_CREATURE_ID="${CREATURE_ID[$ns]}"
  export DECILLION_${key}_PROGRAM_ID="${PROGRAM_ID[$ns]}"
  export DECILLION_${key}_ENTITY="main"
done

set +e
echo
echo "--- stores.create ---"
decillion --batch "stores.create true true global my-first-space" 2>&1 | tail -3
echo
echo "--- stores.list ---"
decillion --batch "stores.list 0 10" 2>&1 | tail -3

echo
echo "--- exporting env for follow-up tests ---"
for ns in "${!WASMS[@]}"; do
  key="${NS_TO_KEY[$ns]}"
  echo "export DECILLION_${key}_CREATURE_ID=${CREATURE_ID[$ns]} DECILLION_${key}_PROGRAM_ID=${PROGRAM_ID[$ns]} DECILLION_${key}_ENTITY=main"
done > /tmp/decillion-deploy-env.sh
chmod +x /tmp/decillion-deploy-env.sh
cat /tmp/decillion-deploy-env.sh
