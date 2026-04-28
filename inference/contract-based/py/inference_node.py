"""
Inference node daemon — contract-based version.

Polls the InferenceMarketplace contract for InferenceRequested events,
runs CommitLLM inference, and calls submitResponse() with:
  - outputHash, modelCommitment (W_root), xRoot, zRoot in calldata
  - xEncoded, zEncoded (binary float32) emitted as event data

Usage:
  python inference_node.py <private_key> <model> [thor_url]

Private keys (VeChain solo devnet):
  inference: 7b067f53d350f1cf20ec13df416b7b73e88a1dc7331bc904b92108b1e76a08b1
"""
from __future__ import annotations

import hashlib
import logging
import sys
import time
from typing import Optional

sys.path.insert(0, "/app")

from contract_client import ContractClient
from commitllm.receipt import Receipt

log = logging.getLogger("inference_node")

POLL_INTERVAL = 5.0  # seconds


class InferenceNode:
    def __init__(self, pk: str, model_name: str, thor_url: str = "http://localhost:8669"):
        self.pk         = pk
        self.model_name = model_name
        self.client     = ContractClient(thor_url)
        self._prover    = None
        self._seen: set[str] = set()

    def _prover_instance(self):
        if self._prover is None:
            from commitllm.prover import CommitLLMProver
            log.info("Loading model %s …", self.model_name)
            self._prover = CommitLLMProver(self.model_name)
            log.info("Model loaded. W_root = %s…", self._prover.model_commitment[:18])
        return self._prover

    def _handle_request(self, event: dict) -> None:
        rid_hex = event["requestId"]
        if rid_hex in self._seen:
            return
        self._seen.add(rid_hex)

        if event["model"] != self.model_name:
            return

        log.info("[%s] Running inference …", rid_hex[:10])
        prover = self._prover_instance()

        # Placeholder prompt — real prompt is delivered off-chain.
        # The user sends the prompt directly to the inference node over HTTPS.
        # On-chain we only have inputHash = sha256(prompt).
        prompt = f"[request {rid_hex}]"  # production: receive via off-chain channel
        output, receipt = prover.generate_with_proof(prompt, max_new_tokens=100)

        # Compute x_root, z_root from the layer trace
        layer_name = next(iter(receipt.layer_traces))
        trace = receipt.layer_traces[layer_name]
        x_root, z_root, x_encoded, z_encoded = prover.compute_roots(trace)

        output_hash = bytes.fromhex(receipt.output_hash)
        model_commitment = receipt.model_commitment  # 0x + hex W_root

        log.info("[%s] output: %r", rid_hex[:10], output[:60])
        log.info("[%s] W_root: %s…", rid_hex[:10], model_commitment[:18])

        request_id_bytes = bytes.fromhex(rid_hex[2:] if rid_hex.startswith("0x") else rid_hex)

        tx_id = self.client.submit_response(
            self.pk, request_id_bytes,
            output_hash, model_commitment,
            x_root, z_root,
            x_encoded, z_encoded,
        )
        log.info("[%s] submitResponse tx = %s", rid_hex[:10], tx_id)

    def run(self, from_block: int = 0) -> None:
        log.info("Inference node started (model=%s)", self.model_name)
        current = from_block
        sig = "InferenceRequested(bytes32,address,string,bytes32,bytes32,uint32,uint256)"

        while True:
            try:
                best = int(self.client.thor.best_block()["number"])
                if best >= current:
                    events = self.client.get_events(sig, current, best)
                    for log_entry in events:
                        evt = self.client.parse_inference_requested(log_entry)
                        self._handle_request(evt)
                    current = best + 1
            except Exception as exc:
                log.warning("Poll error: %s", exc)
            time.sleep(POLL_INTERVAL)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(message)s")
    pk       = sys.argv[1] if len(sys.argv) > 1 else "7b067f53d350f1cf20ec13df416b7b73e88a1dc7331bc904b92108b1e76a08b1"
    model    = sys.argv[2] if len(sys.argv) > 2 else "Qwen/Qwen2.5-1.5B-Instruct"
    thor_url = sys.argv[3] if len(sys.argv) > 3 else "http://localhost:8669"
    InferenceNode(pk, model, thor_url).run()
