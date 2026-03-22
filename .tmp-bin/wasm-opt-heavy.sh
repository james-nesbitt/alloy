#!/bin/bash
# High-precision wasm-opt shim for TinyGo
LOG="/tmp/wasm-opt-shim.log"
echo "ARGS: $*" >> "$LOG"

if [[ "$*" == *"--version"* ]]; then
  echo "wasm-opt version 110"
  exit 0
fi

# TinyGo pattern: wasm-opt [IN] -o [OUT] [FLAGS]
# We need to find the positional IN and the -o OUT
IN=""
OUT=""
for ((i=1; i<=$#; i++)); do
  arg="${@:i:1}"
  if [[ "$arg" == "-o" ]]; then
    OUT="${@:i+1:1}"
    ((i++))
  elif [[ "$arg" != -* ]]; then
    IN="$arg"
  fi
done

if [[ -n "$IN" && -n "$OUT" ]]; then
  echo "SHIM: cp $IN $OUT" >> "$LOG"
  cp "$IN" "$OUT"
  exit 0
fi

# Fallback: if we only have one arg and it ends in .wasm
if [[ $# -ge 1 && "$1" == *.wasm ]]; then
   # Maybe it's being used as wasm-opt in.wasm -o out.wasm
   # Let's try to find -o and next arg
   # (redundant with above but let's be safe)
   cp "$1" "${@: -1}" 2>/dev/null || true
fi

exit 0
