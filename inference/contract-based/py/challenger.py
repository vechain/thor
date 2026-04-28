"""
Challenger daemon — submits bisection fraud proofs.

When a checker's Freivalds check fails, the challenger:
  1. Loads the model weights locally
  2. Computes the full matrix product W·x
  3. Finds the first index i where W[i]·x ≠ z[i]
  4. Generates Merkle proofs for W[i], x[i], z[i]
  5. Calls submitFraudProof() — slashes the inference node

Anyone with the model weights can act as challenger (universal challenger model:
one honest actor globally is sufficient to break collusion).

Usage:
  python challenger.py <private_key> [thor_url]

Private keys (VeChain solo devnet):
  user (default challenger): 99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36
"""
from __future__ import annotations

import logging
import sys
import time
from typing import Optional

sys.path.insert(0, "/app")

from contract_client import ContractClient
from commitllm.merkle import (
    compute_vector_root, compute_w_root,
    get_w_row_proof, get_element_proof,
    decode_float32, SCALE,
)

log = logging.getLogger("challenger")

POLL_INTERVAL = 5.0
CHALLENGER_BOND = 10**18  # 1 VET bond


class ChallengerNode:
    def __init__(self, pk: str, thor_url: str = "http://localhost:8669"):
        self.pk     = pk
        self.client = ContractClient(thor_url)
        self._models: dict[str, object] = {}  # model_name → loaded weights
        self._seen: set[str] = set()

    def _get_weights(self, model_name: str):
        """Load and cache model weights for fraud proof computation."""
        if model_name not in self._models:
            from transformers import AutoModelForCausalLM
            import torch
            log.info("Loading weights for %s …", model_name)
            model = AutoModelForCausalLM.from_pretrained(
                model_name, torch_dtype=torch.float16, device_map="cpu"
            )
            model.eval()
            # Find hook layer (middle o_proj)
            layers = [
                (n, m) for n, m in model.named_modules()
                if n.endswith(".o_proj") and isinstance(m, torch.nn.Linear)
            ]
            if not layers:
                layers = [
                    (n, m) for n, m in model.named_modules()
                    if isinstance(m, torch.nn.Linear) and not n.endswith("lm_head")
                ]
            k = len(layers)
            name, module = layers[k // 2]
            W = module.weight.detach().cpu()
            self._models[model_name] = {"W": W, "name": name}
            log.info("Weights loaded: layer=%s, shape=%s", name, tuple(W.shape))
        return self._models[model_name]

    def _find_disputed_index(self, W, x_list: list[float], z_list: list[float]) -> Optional[int]:
        """Find first index i where round(W[i]·x * SCALE) ≠ z_i_scaled."""
        import torch
        import numpy as np
        W32 = W.to(torch.float32)
        x = torch.tensor(x_list, dtype=torch.float32)
        z = torch.tensor(z_list, dtype=torch.float32)

        y = (W32 @ x).tolist()

        tol = 0.01  # 1% relative tolerance for fp16 accumulation
        for i in range(len(z_list)):
            scale = max(abs(y[i]), abs(z_list[i]), 1e-6)
            if abs(y[i] - z_list[i]) / scale > tol:
                return i
        return None

    def _handle_fraud(self, evt: dict, model_name: str) -> None:
        rid_hex = evt["requestId"]
        log.info("[%s] FRAUD detected — building fraud proof …", rid_hex[:10])

        x = decode_float32(evt["xEncoded"])
        z = decode_float32(evt["zEncoded"])

        weights = self._get_weights(model_name)
        W = weights["W"]

        disputed_i = self._find_disputed_index(W, x, z)
        if disputed_i is None:
            log.warning("[%s] Could not find disputed index — no fraud proof", rid_hex[:10])
            return

        log.info("[%s] Disputed index: %d", rid_hex[:10], disputed_i)

        # Build Merkle proofs
        w_row_bytes, w_proof = get_w_row_proof(W, disputed_i)
        x_i_scaled, x_proof  = get_element_proof(x, disputed_i)
        z_i_scaled, z_proof  = get_element_proof(z, disputed_i)

        request_id_bytes = bytes.fromhex(rid_hex[2:] if rid_hex.startswith("0x") else rid_hex)

        tx_id = self.client.submit_fraud_proof(
            self.pk, request_id_bytes, disputed_i,
            w_row_bytes,
            w_proof, x_proof, z_proof,
            x_i_scaled, z_i_scaled,
            bond_wei=CHALLENGER_BOND,
        )
        log.info("[%s] submitFraudProof tx = %s", rid_hex[:10], tx_id)

    def _handle_response(self, evt: dict) -> None:
        rid_hex = evt["requestId"]
        if rid_hex in self._seen:
            return
        self._seen.add(rid_hex)

        # Get model name from the corresponding request event
        req_events = self.client.get_events(
            "InferenceRequested(bytes32,address,string,bytes32,bytes32,uint32,uint256)"
        )
        model_name = None
        for log_entry in req_events:
            parsed = self.client.parse_inference_requested(log_entry)
            if parsed["requestId"] == rid_hex:
                model_name = parsed["model"]
                break

        if model_name is None:
            return

        try:
            weights = self._get_weights(model_name)
            x = decode_float32(evt["xEncoded"])
            z = decode_float32(evt["zEncoded"])
            disputed_i = self._find_disputed_index(weights["W"], x, z)
            if disputed_i is not None:
                self._handle_fraud(evt, model_name)
        except Exception as exc:
            log.warning("[%s] Challenger error: %s", rid_hex[:10], exc)

    def run(self, from_block: int = 0) -> None:
        log.info("Challenger node started")
        current = from_block
        sig = "InferenceResponded(bytes32,address,bytes32,bytes32,bytes32,bytes32,bytes,bytes)"

        while True:
            try:
                best = int(self.client.thor.best_block()["number"])
                if best >= current:
                    events = self.client.get_events(sig, current, best)
                    for log_entry in events:
                        evt = self.client.parse_inference_responded(log_entry)
                        self._handle_response(evt)
                    current = best + 1
            except Exception as exc:
                log.warning("Poll error: %s", exc)
            time.sleep(POLL_INTERVAL)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(message)s")
    pk       = sys.argv[1] if len(sys.argv) > 1 else "99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36"
    thor_url = sys.argv[2] if len(sys.argv) > 2 else "http://localhost:8669"
    ChallengerNode(pk, thor_url).run()
