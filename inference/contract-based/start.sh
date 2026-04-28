#!/usr/bin/env bash
# start.sh — spin up the full InferenceMarketplace V1 stack
#
# Steps:
#   1. Start VeChain solo node (Docker)
#   2. npm install + Hardhat compile → produces bytecode artifact
#   3. Python deploy → writes deployment.json (VeChain signer, not Hardhat)
#   4. Python seed → registerModel + registerInferenceNode + registerCheckerNode
#   5. Start inference_node.py locally (uses MPS/GPU if available)
#   6. Start checker_node.py locally
#   7. Start explorer (Docker) → dashboard at http://localhost:8088
#
# Note on Hardhat + VeChain:
#   Hardhat compiles the Solidity → artifacts/. It cannot deploy to VeChain
#   because VeChain's chain ID is a 256-bit genesis block ID that Hardhat's
#   ethers signer cannot handle. Python deploy.py handles deployment instead.
#
# Prerequisites:
#   - Docker Desktop running
#   - Node.js >= 18 with npm
#   - Python >= 3.11 with venv at ../inference/.venv (or active venv)
#   - HuggingFace model cached: Qwen/Qwen2.5-1.5B-Instruct

set -euo pipefail
cd "$(dirname "$0")"

THOR_URL="${THOR_URL:-http://localhost:8669}"
MODEL="${MODEL:-Qwen/Qwen2.5-1.5B-Instruct}"
INFERENCE_PK="${INFERENCE_PK:-7b067f53d350f1cf20ec13df416b7b73e88a1dc7331bc904b92108b1e76a08b1}"
CHECKER_PK="${CHECKER_PK:-f4a1a17039216f535d42ec23732c79943ffb45a089fbb78a14daad0dae93e991}"

# ── Python virtualenv ──────────────────────────────────────────────────────── #
VENV_PYTHON=""
if [ -f "../.venv/bin/python" ]; then
  VENV_PYTHON="../.venv/bin/python"
elif [ -n "${VIRTUAL_ENV:-}" ]; then
  VENV_PYTHON="python"
else
  echo "ERROR: No Python venv found. Run: python -m venv ../.venv && ../.venv/bin/pip install -r commitllm/requirements.txt -r py/requirements.txt"
  exit 1
fi

echo "=== [1/7] Starting VeChain solo node ==="
docker compose up thor -d
echo "Waiting for Thor to be healthy …"
until docker compose exec thor wget -qO- http://localhost:8669/blocks/best >/dev/null 2>&1; do
  sleep 2
done
echo "Thor is ready."

echo ""
echo "=== [2/7] Compiling Solidity ==="
if [ ! -d "node_modules" ]; then
  echo "Running npm install …"
  npm install
fi
npx hardhat compile
echo "Compilation complete. Artifact: artifacts/contracts/InferenceMarketplace.sol/InferenceMarketplace.json"

echo ""
echo "=== [3/7] Deploying contract ==="
PYTHONPATH="py:." $VENV_PYTHON scripts/deploy.py "$THOR_URL"
echo "Contract deployed. See deployment.json"

echo ""
echo "=== [4/7] Seeding contract (register model + nodes) ==="
PYTHONPATH="py:." $VENV_PYTHON scripts/seed.py "$THOR_URL" "$MODEL"

echo ""
echo "=== [5/7] Starting inference node (local) ==="
PYTHONPATH="py:." $VENV_PYTHON py/inference_node.py "$INFERENCE_PK" "$MODEL" "$THOR_URL" &
INFERENCE_PID=$!
echo "Inference node PID: $INFERENCE_PID"

echo ""
echo "=== [6/7] Starting checker node (local) ==="
PYTHONPATH="py:." $VENV_PYTHON py/checker_node.py "$CHECKER_PK" "$THOR_URL" &
CHECKER_PID=$!
echo "Checker node PID: $CHECKER_PID"

echo ""
echo "=== [7/7] Starting Inference Explorer ==="
docker compose up explorer -d
echo "Explorer available at http://localhost:8088"

echo ""
echo "============================================================"
echo "  Stack is running!"
echo ""
echo "  Explorer:    http://localhost:8088"
echo "  Thor RPC:    $THOR_URL"
echo "  Model:       $MODEL"
echo ""
echo "  Inference PID: $INFERENCE_PID"
echo "  Checker PID:   $CHECKER_PID"
echo ""
echo "  To submit a test request:"
echo "    PYTHONPATH=py:. $VENV_PYTHON -c \""
echo "    from contract_client import ContractClient"
echo "    import hashlib"
echo "    c = ContractClient('$THOR_URL')"
echo "    pk = '99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36'"
echo "    input_hash = bytes.fromhex(hashlib.sha256(b'What is 2+2?').hexdigest())"
echo "    tx = c.submit_request(pk, '$MODEL', input_hash, b'\\x00'*32, 100)"
echo "    print('Request tx:', tx)"
echo "    \""
echo ""
echo "  To stop: kill $INFERENCE_PID $CHECKER_PID && docker compose down"
echo "============================================================"

# Wait so the script keeps running; Ctrl-C shuts down daemons
wait $INFERENCE_PID $CHECKER_PID
