#!/bin/bash
if [[ "$*" == "--version" ]]; then
  echo "wasm-opt version 110"
  exit 0
fi

INPUT=""
OUTPUT=""
ARGS=("$@")
for ((i=0; i<${#ARGS[@]}; i++)); do
  case "${ARGS[i]}" in
    -o|--output)
      OUTPUT="${ARGS[i+1]}"
      ((i++))
      ;;
    -*)
      ;;
    *)
      INPUT="${ARGS[i]}"
      ;;
  esac
done

if [[ -n "$INPUT" && -n "$OUTPUT" ]]; then
  cp "$INPUT" "$OUTPUT"
fi
exit 0
