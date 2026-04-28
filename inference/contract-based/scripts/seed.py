"""
Seed the InferenceMarketplace contract with initial state:
  1. Register the inference model (Qwen/Qwen2.5-1.5B-Instruct) + W_root
  2. Register the inference node  (stake 10 VET)
  3. Register the checker node    (stake 5 VET)

W_root = MerkleRoot(rows of hook-layer weight matrix W).
This is computed from locally downloaded model weights.

Usage:
  python scripts/seed.py [thor_url] [model_name]

Prerequisites:
  - VeChain solo node running
  - deployment.json written by deploy.py
  - Model weights downloaded (HuggingFace cache)
"""
from __future__ import annotations

import json
import os
import sys
import time

BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.join(BASE_DIR, "py"))
sys.path.insert(0, BASE_DIR)

from contract_client import ContractClient

# ── Dev accounts (VeChain solo devnet genesis) ────────────────────────────── #
ACCOUNTS = {
    "user":      "99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36",
    "inference": "7b067f53d350f1cf20ec13df416b7b73e88a1dc7331bc904b92108b1e76a08b1",
    "checker":   "f4a1a17039216f535d42ec23732c79943ffb45a089fbb78a14daad0dae93e991",
}

DEFAULT_MODEL = "Qwen/Qwen2.5-1.5B-Instruct"

INFERENCE_STAKE = 10 * 10**18  # 10 VET
CHECKER_STAKE   = 5  * 10**18  # 5 VET


def wait_tx(client: ContractClient, tx_id: str, label: str) -> None:
    print(f"  {label}: {tx_id} …", end="", flush=True)
    client.wait_tx(tx_id, timeout=60)
    print(" confirmed")


def compute_w_root(model_name: str) -> str:
    """Load model, extract hook layer, compute W_root."""
    print(f"  Loading {model_name} to compute W_root …")
    from commitllm.prover import CommitLLMProver
    prover = CommitLLMProver(model_name)
    w_root = prover.model_commitment
    print(f"  W_root = {w_root[:18]}…")
    return w_root


def main() -> None:
    thor_url   = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8669"
    model_name = sys.argv[2] if len(sys.argv) > 2 else DEFAULT_MODEL

    client = ContractClient(thor_url)
    print(f"Contract address: {client.address}")
    print(f"Chain block:      #{client.thor.best_block()['number']}")
    print()

    # 1. Register model
    print(f"[1/3] Registering model: {model_name}")
    w_root = compute_w_root(model_name)
    tx_id = client.register_model(ACCOUNTS["user"], model_name, w_root)
    wait_tx(client, tx_id, "registerModel")

    # Verify
    wroot_on_chain, registered = client.get_model(model_name)
    assert registered, "model not registered!"
    assert wroot_on_chain.lower() == w_root.lower(), \
        f"W_root mismatch: {wroot_on_chain} vs {w_root}"
    print(f"  ✓ registered, W_root matches")
    print()

    # 2. Register inference node
    print("[2/3] Registering inference node (stake: 10 VET)")
    tx_id = client.register_inference_node(ACCOUNTS["inference"], INFERENCE_STAKE)
    wait_tx(client, tx_id, "registerInferenceNode")

    from eth_keys import keys as eth_keys
    inf_addr = eth_keys.PrivateKey(
        bytes.fromhex(ACCOUNTS["inference"])
    ).public_key.to_address()
    stake_wei, active = client.get_inference_node(inf_addr)
    assert active, "inference node not active!"
    print(f"  ✓ active, stake = {stake_wei / 10**18:.1f} VET")
    print()

    # 3. Register checker node
    print("[3/3] Registering checker node (stake: 5 VET)")
    tx_id = client.register_checker_node(ACCOUNTS["checker"], CHECKER_STAKE)
    wait_tx(client, tx_id, "registerCheckerNode")

    chk_addr = eth_keys.PrivateKey(
        bytes.fromhex(ACCOUNTS["checker"])
    ).public_key.to_address()
    stake_wei, active = client.get_checker_node(chk_addr)
    assert active, "checker node not active!"
    print(f"  ✓ active, stake = {stake_wei / 10**18:.1f} VET")
    print()

    print("Seed complete. Ready to run inference_node.py and checker_node.py.")


if __name__ == "__main__":
    main()
