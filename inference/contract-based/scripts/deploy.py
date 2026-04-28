"""
Deploy InferenceMarketplace.sol to VeChain solo node.

Reads the compiled artifact from artifacts/contracts/InferenceMarketplace.sol/
and deploys it using vechain_client (Python signer) — not Hardhat's deploy task,
because VeChain's 256-bit chain ID is incompatible with Hardhat's ethers signer.

Writes deployment.json to the contract-based/ root with:
  { "address": "0x...", "txId": "0x...", "blockNumber": N }

Usage:
  python scripts/deploy.py [thor_url]
"""
from __future__ import annotations

import json
import os
import sys
import time

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "py"))

from vechain_client import ThorClient

BASE_DIR    = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ARTIFACT    = os.path.join(
    BASE_DIR, "artifacts", "contracts",
    "InferenceMarketplace.sol", "InferenceMarketplace.json"
)
OUTPUT_FILE = os.path.join(BASE_DIR, "deployment.json")

# Deployer key (devnet genesis account 4 — 1B VET, used for deployment)
DEPLOYER_PK = "35b5cc144faca7d7f220fca7ad3420090861d5231d80eb23e1013426847371c4"


def load_bytecode() -> str:
    with open(ARTIFACT) as f:
        artifact = json.load(f)
    return artifact["bytecode"]


def derive_contract_address(tx_id: str, client: ThorClient) -> str:
    """
    Derive the deployed contract address from the transaction receipt.
    VeChain puts the contract address in the transaction's output[0].contractAddress.
    """
    deadline = time.time() + 60.0
    while time.time() < deadline:
        try:
            tx = client._get(f"/transactions/{tx_id}/receipt")
            if tx and tx.get("outputs"):
                addr = tx["outputs"][0].get("contractAddress")
                if addr:
                    return addr
        except Exception:
            pass
        time.sleep(2)
    raise TimeoutError(f"Could not get contract address from tx {tx_id}")


def main() -> None:
    thor_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8669"
    client   = ThorClient(thor_url)

    print(f"Connecting to {thor_url} …")
    best = client.best_block()
    print(f"Chain tag: 0x{client.chain_tag():02x}  Block: #{best['number']}")

    print("Loading artifact …")
    bytecode = load_bytecode()
    deploy_data = bytes.fromhex(bytecode[2:] if bytecode.startswith("0x") else bytecode)

    print("Deploying InferenceMarketplace …")
    tx_id = client.send_transaction(
        DEPLOYER_PK,
        clauses=[(None, 0, deploy_data)],
        gas=3_000_000,
    )
    print(f"Deploy tx: {tx_id}")

    print("Waiting for confirmation …")
    contract_address = derive_contract_address(tx_id, client)
    print(f"Contract deployed at: {contract_address}")

    receipt_info = client._get(f"/transactions/{tx_id}/receipt")
    block_num    = receipt_info.get("meta", {}).get("blockNumber", 0)

    deployment = {
        "address":     contract_address,
        "txId":        tx_id,
        "blockNumber": block_num,
        "thor_url":    thor_url,
    }
    with open(OUTPUT_FILE, "w") as f:
        json.dump(deployment, f, indent=2)
    print(f"Wrote {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
