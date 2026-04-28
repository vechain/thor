#!/bin/bash
# Start the malicious inference node locally (uses Apple Silicon MPS when available).
#
# Alternates between two fraud types on successive requests:
#   wrong-model      — runs 0.5B, claims 1.5B  → FRAUD (commitment mismatch)
#   fake-activations — correct commitment, random activations → FRAUD (Freivalds)
#
# Reuses the .venv created by start_local.sh (or creates one if absent).
#
# Usage:
#   ./start_malicious.sh                                        # defaults
#   ./start_malicious.sh <private_key> <claimed_model> <actual_model> <thor_url>
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
VENV="$DIR/.venv"

if [ ! -d "$VENV" ]; then
    echo "==> Creating Python virtual environment at $VENV …"
    python3 -m venv "$VENV"
    "$VENV/bin/pip" install --upgrade pip --quiet
    echo "==> Installing dependencies …"
    "$VENV/bin/pip" install \
        "torch>=2.0.0" \
        "transformers>=4.40.0" \
        "accelerate>=0.27.0" \
        "eth-keys>=0.4.0" \
        "requests>=2.28.0"
    echo "==> Dependencies installed."
fi

PRIVATE_KEY="${1:-35b5cc144faca7d7f220fca7ad3420090861d5231d80eb23e1013426847371c4}"
CLAIMED_MODEL="${2:-Qwen/Qwen2.5-1.5B-Instruct}"
ACTUAL_MODEL="${3:-Qwen/Qwen2.5-0.5B-Instruct}"
THOR_URL="${4:-http://localhost:8669}"

echo "==> Starting malicious inference node"
echo "    claimed  : $CLAIMED_MODEL"
echo "    actual   : $ACTUAL_MODEL"
echo "    thor     : $THOR_URL"
echo "    key      : ${PRIVATE_KEY:0:8}…"
echo ""
echo "    Fraud types (alternating per request):"
echo "      odd  requests → wrong-model      (commitment mismatch)"
echo "      even requests → fake-activations (Freivalds fails)"
echo ""

PYTHONPATH="$DIR/py:$DIR" \
HF_HOME="$HOME/.cache/huggingface" \
exec "$VENV/bin/python" "$DIR/py/malicious_node.py" \
    "$PRIVATE_KEY" \
    "$CLAIMED_MODEL" \
    "$ACTUAL_MODEL" \
    "$THOR_URL"
