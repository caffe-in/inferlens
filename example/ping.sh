#!/usr/bin/env bash

set -euo pipefail

MODEL="${MODEL:-Qwen/Qwen2.5-0.5B-Instruct}"
PROMPT="${PROMPT:-hello}"
ENDPOINT="${ENDPOINT:-http://localhost:8000}"

./artifacts/inferlens ping serve \
  --endpoint "$ENDPOINT" \
  --model "$MODEL" \
  --prompt "$PROMPT"
