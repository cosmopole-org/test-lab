#!/bin/bash
# Build all WASM creatures using TinyGo for the host's wasm/wasmedge runtime.
# Outputs go into ./wasm/<namespace>[/endpoints/<endpoint>].wasm

set -e

ROOT="$(cd "$(dirname "$0")" && pwd)"
OUT_ROOT="$ROOT/wasm"
mkdir -p "$OUT_ROOT"

# Build a creature into a single .wasm file.
# $1: path to creature dir (containing go.mod + main.go)
# $2: output .wasm path
build_one() {
  local src="$1"
  local out="$2"
  mkdir -p "$(dirname "$out")"
  echo "[wasm] $src -> $out"
  (cd "$src" && tinygo build -o "$out" -target=wasi -no-debug -opt=2 -scheduler=none .)
}

# Namespace creatures (top-level)
for ns in chain invites pc storage stores; do
  build_one "$ROOT/creatures/$ns" "$OUT_ROOT/$ns.wasm"
done

# Endpoint creatures (one wasm per endpoint)
while IFS= read -r mod; do
  src_dir="$(dirname "$mod")"
  rel="${src_dir#$ROOT/creatures/endpoints/}"
  out="$OUT_ROOT/endpoints/$rel.wasm"
  build_one "$src_dir" "$out"
done < <(find "$ROOT/creatures/endpoints" -name go.mod | sort)

echo "All WASM artifacts built under $OUT_ROOT/"
find "$OUT_ROOT" -name '*.wasm' | sort
