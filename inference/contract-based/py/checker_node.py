"""
Checker node daemon — contract-based version.

Polls InferenceResponded events, reads x/z from event data, runs Freivalds
with deterministic r vectors, and calls submitVerdict().

Fraud detection:
  1. Model commitment check: W_root in event must match verifier's local W_root
  2. Freivalds: r·z ≈ r·W·x  (deterministic r from on-chain entropy)

Usage:
  python checker_node.py <private_key> [thor_url]

Private keys (VeChain solo devnet):
  checker: f4a1a17039216f535d42ec23732c79943ffb45a089fbb78a14daad0dae93e991
"""
from __future__ import annotations

import logging
import sys
import time
from typing import Optional

sys.path.insert(0, "/app")

from contract_client import ContractClient
from commitllm.merkle import decode_float32

log = logging.getLogger("checker_node")

POLL_INTERVAL = 5.0


class CheckerNode:
    def __init__(self, pk: str, thor_url: str = "http://localhost:8669"):
        self.pk      = pk
        self.client  = ContractClient(thor_url)
        self._verifiers: dict[str, object] = {}
        self._seen: set[str] = set()

    def _get_verifier(self, model_name: str):
        if model_name not in self._verifiers:
            from commitllm.verifier import CommitLLMVerifier
            log.info("Loading verifier for %s …", model_name)
            self._verifiers[model_name] = CommitLLMVerifier(model_name)
            log.info("Verifier ready. W_root = %s…",
                     self._verifiers[model_name].model_commitment[:18])
        return self._verifiers[model_name]

    def _handle_response(self, event: dict) -> None:
        rid_hex = event["requestId"]
        if rid_hex in self._seen:
            return
        self._seen.add(rid_hex)

        log.info("[%s] Verifying response …", rid_hex[:10])

        try:
            # Decode x/z from binary float32 event data
            x = decode_float32(event["xEncoded"])
            z = decode_float32(event["zEncoded"])
            model_commitment = event["modelCommitment"]

            # Get block hash for deterministic r (use response block)
            block_num = event["blockNumber"]
            block_info = self.client.thor.get_block_header(str(block_num))
            block_hash = bytes.fromhex(block_info["id"][2:])
            request_id_bytes = bytes.fromhex(rid_hex[2:] if rid_hex.startswith("0x") else rid_hex)

            # Check model commitment first (fast path)
            req_event = self.client.get_events(
                "InferenceRequested(bytes32,address,string,bytes32,bytes32,uint32,uint256)"
            )
            model_name = None
            for log_entry in req_event:
                parsed = self.client.parse_inference_requested(log_entry)
                if parsed["requestId"] == rid_hex:
                    model_name = parsed["model"]
                    break

            if model_name is None:
                log.warning("[%s] Could not find matching request event", rid_hex[:10])
                return

            from commitllm.receipt import Receipt
            import torch

            verifier = self._get_verifier(model_name)
            if model_commitment != verifier.model_commitment:
                ok, reason = False, (
                    f"model commitment mismatch: "
                    f"event={model_commitment[:18]}... expected={verifier.model_commitment[:18]}..."
                )
            else:
                # Run Freivalds with deterministic r vectors
                W = verifier._get_layer_weight(
                    self._infer_layer_name(verifier, model_name)
                )
                from commitllm.verifier import freivalds_check
                ok, reason = freivalds_check(
                    W.to(torch.float32),
                    torch.tensor(x, dtype=torch.float32),
                    torch.tensor(z, dtype=torch.float32),
                    request_id=request_id_bytes,
                    block_hash=block_hash,
                )

        except Exception as exc:
            ok, reason = False, f"verification error: {exc}"

        verdict = "VALID" if ok else "FRAUD"
        log.info("[%s] Verdict: %s — %s", rid_hex[:10], verdict, reason[:80])

        request_id_bytes = bytes.fromhex(rid_hex[2:] if rid_hex.startswith("0x") else rid_hex)
        tx_id = self.client.submit_verdict(self.pk, request_id_bytes, ok, reason)
        log.info("[%s] submitVerdict tx = %s", rid_hex[:10], tx_id)

    def _infer_layer_name(self, verifier, model_name: str) -> str:
        """Get the hook layer name (middle o_proj) from the verifier's model."""
        layers = [
            n for n, m in verifier.model.named_modules()
            if n.endswith(".o_proj")
        ]
        if not layers:
            layers = [
                n for n, m in verifier.model.named_modules()
                if hasattr(m, "weight") and not n.endswith("lm_head")
            ]
        return layers[len(layers) // 2] if layers else ""

    def run(self, from_block: int = 0) -> None:
        log.info("Checker node started")
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
    pk       = sys.argv[1] if len(sys.argv) > 1 else "f4a1a17039216f535d42ec23732c79943ffb45a089fbb78a14daad0dae93e991"
    thor_url = sys.argv[2] if len(sys.argv) > 2 else "http://localhost:8669"
    CheckerNode(pk, thor_url).run()
