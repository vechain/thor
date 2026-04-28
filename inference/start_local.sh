#!/bin/bash
# Start the inference node locally (uses Apple Silicon MPS when available).
#
# First run creates .venv and installs deps (~2 min, once only).
# Models are read from ~/Library/Caches/huggingface (already downloaded).
#
# Usage:
#   ./start_local.sh                           # default key + model + thor at localhost:8669
#   ./start_local.sh <private_key> <model> <thor_url>
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
VENV="$DIR/.venv"

if [ ! -d "$VENV" ]; then
    echo "==> Creating Python virtual environment at $VENV …"
    python3 -m venv "$VENV"
    "$VENV/bin/pip" install --upgrade pip --quiet
    echo "==> Installing dependencies (torch, transformers, accelerate, eth-keys, requests) …"
    "$VENV/bin/pip" install \
        "torch>=2.0.0" \
        "transformers>=4.40.0" \
        "accelerate>=0.27.0" \
        "eth-keys>=0.4.0" \
        "requests>=2.28.0"
    echo "==> Dependencies installed."
fi

PRIVATE_KEY="${1:-7b067f53d350f1cf20ec13df416b7b73e88a1dc7331bc904b92108b1e76a08b1}"
MODEL="${2:-Qwen/Qwen2.5-1.5B-Instruct}"
THOR_URL="${3:-http://localhost:8669}"

echo "==> Starting inference node"
echo "    model    : $MODEL"
echo "    thor     : $THOR_URL"
echo "    key      : ${PRIVATE_KEY:0:8}…"
echo ""

PYTHONPATH="$DIR/py:$DIR" \
HF_HOME="$HOME/.cache/huggingface" \
exec "$VENV/bin/python" "$DIR/py/inference_node.py" \
    "$PRIVATE_KEY" \
    "$MODEL" \
    "$THOR_URL"
