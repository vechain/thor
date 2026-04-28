"""
Malicious inference node — demonstrates Byzantine behaviour for PoC.

Alternates between two fraud types on successive requests:

  Type 1 (wrong-model):      runs Qwen2.5-0.5B internally, claims 1.5B.
                              Receipt carries 0.5B commitment → checker sees
                              commitment mismatch → FRAUD (Step 1).

  Type 2 (fake-activations): uses the correct 1.5B commitment (learned from
                              an honest INRS already on-chain) but replaces
                              x/z in the receipt with random Gaussian vectors.
                              Commitment check passes; Freivalds r·z ≠ r·W·x
                              → FRAUD (Step 2).

Both fraud types use the 0.5B model to generate the visible output text so
the response looks like a real LLM answer. Only the on-chain proof reveals
the fraud.

Usage:
  python malicious_node.py <private_key> <claimed_model> <actual_model> [thor_url]

  private_key    secp256k1 private key hex (no 0x prefix)
  claimed_model  model name announced in INRS  (e.g. Qwen/Qwen2.5-1.5B-Instruct)
  actual_model   model actually loaded locally (e.g. Qwen/Qwen2.5-0.5B-Instruct)
  thor_url       VeChain node URL (default: http://localhost:8669)
"""

from __future__ import annotations

import hashlib
import json
import logging
import random
import sys
import time
from typing import Optional

from protocol import (
    INFERENCE_ADDR,
    InferenceRequest,
    InferenceResponse,
    decode_message,
    encode_response,
)
from vechain_client import ThorClient

sys.path.insert(0, "/app")  # commitllm package in Docker

log = logging.getLogger("malicious_node")

_FRAUD_MODES = ["wrong-model", "fake-activations"]


class MaliciousInferenceNode:
    def __init__(
        self,
        private_key_hex: str,
        claimed_model: str,
        actual_model: str,
        thor_url: str = "http://thor:8669",
        poll_interval: float = 5.0,
    ):
        self.pk            = private_key_hex
        self.claimed_model = claimed_model
        self.actual_model  = actual_model
        self.client        = ThorClient(thor_url)
        self.poll_interval = poll_interval
        self._prover       = None
        self._seen_requests: set[str] = set()
        self._request_counter: int = 0

        # Learned from an honest INRS already on-chain
        self._claimed_commitment: Optional[str] = None
        self._layer_name: Optional[str] = None
        self._layer_dim:  Optional[int] = None

    # ── Model loading ─────────────────────────────────────────────────────── #

    def _prover_instance(self):
        if self._prover is None:
            from commitllm.prover import CommitLLMProver
            log.info("Loading actual model %s …", self.actual_model)
            self._prover = CommitLLMProver(self.actual_model)
            log.info(
                "Actual model loaded (commitment %s…)",
                self._prover.model_commitment[:16],
            )
        return self._prover

    # ── Commitment learning ───────────────────────────────────────────────── #

    def _extract_commitment(self, msg: InferenceResponse) -> None:
        """Parse an honest INRS receipt to learn the claimed model's commitment."""
        if self._claimed_commitment is not None:
            return
        if msg.model != self.claimed_model:
            return
        try:
            r      = json.loads(msg.receipt_json)
            commit = r.get("model_commitment", "")
            traces = r.get("layer_traces", {})
            if not commit or not traces:
                return
            layer_name = next(iter(traces))
            layer_dim  = len(traces[layer_name].get("x", []))
            if layer_dim == 0:
                return
            self._claimed_commitment = commit
            self._layer_name         = layer_name
            self._layer_dim          = layer_dim
            log.info(
                "Learned %s commitment from chain: %s… (layer=%s, dim=%d)",
                self.claimed_model, commit[:16], layer_name, layer_dim,
            )
        except Exception as exc:
            log.debug("Could not extract commitment: %s", exc)

    # ── Receipt building ──────────────────────────────────────────────────── #

    def _forge_receipt(self, prompt: str, output: str):
        """Build a receipt with the correct commitment but random activations."""
        from commitllm.receipt import LayerTrace, Receipt
        fake_x = [random.gauss(0.0, 1.0) for _ in range(self._layer_dim)]
        fake_z = [random.gauss(0.0, 1.0) for _ in range(self._layer_dim)]
        return Receipt(
            model_name       = self.claimed_model,
            model_commitment = self._claimed_commitment,
            input_hash       = hashlib.sha256(prompt.encode()).hexdigest(),
            output_hash      = hashlib.sha256(output.encode()).hexdigest(),
            layer_traces     = {self._layer_name: LayerTrace(x=fake_x, z=fake_z)},
        )

    # ── Request handling ──────────────────────────────────────────────────── #

    def _handle_request(self, tx_id: str, req: InferenceRequest) -> None:
        if req.request_id in self._seen_requests:
            return
        self._seen_requests.add(req.request_id)

        if req.model != self.claimed_model:
            return

        # Pick fraud mode; fall back to wrong-model if commitment not yet known
        mode = _FRAUD_MODES[self._request_counter % 2]
        if mode == "fake-activations" and self._claimed_commitment is None:
            log.warning(
                "[%s] Commitment not yet learned — falling back to wrong-model",
                req.request_id[:8],
            )
            mode = "wrong-model"
        self._request_counter += 1

        log.info(
            "[%s] FRAUD MODE: %s | prompt: %r",
            req.request_id[:8], mode, req.prompt[:60],
        )

        # Generate output using the small model (looks like a real response)
        prover       = self._prover_instance()
        output, receipt_0_5b = prover.generate_with_proof(
            req.prompt, max_new_tokens=req.max_new_tokens
        )
        log.info("[%s] Output: %r", req.request_id[:8], output[:80])

        if mode == "wrong-model":
            # Claim 1.5B but the receipt carries the 0.5B commitment
            receipt = receipt_0_5b
        else:
            # Claim 1.5B with correct commitment, but random x/z — Freivalds will fail
            receipt = self._forge_receipt(req.prompt, output)

        resp = InferenceResponse(
            request_id   = req.request_id,
            request_tx   = tx_id,
            model        = self.claimed_model,
            output       = output,
            receipt_json = receipt.to_json(),
        )
        sent_tx = self.client.send_transaction(
            self.pk, clauses=[(INFERENCE_ADDR, 0, encode_response(resp))]
        )
        log.info("[%s] Posted INRS tx %s", req.request_id[:8], sent_tx)

    # ── Main loop ─────────────────────────────────────────────────────────── #

    def run(self, start_block: Optional[int] = None) -> None:
        log.info(
            "Malicious node started | claimed=%s | actual=%s",
            self.claimed_model, self.actual_model,
        )

        best = int(self.client.best_block()["number"])
        if start_block is None:
            start_block = best

        # Scan chain history once to learn the honest node's commitment
        log.info("Scanning blocks 0..%d for honest INRS commitment…", best)
        for _, raw in self.client.scan_blocks_for_messages(0, best):
            msg = decode_message(raw)
            if isinstance(msg, InferenceResponse):
                self._extract_commitment(msg)
            if self._claimed_commitment:
                break

        if self._claimed_commitment:
            log.info("Ready for fake-activations fraud")
        else:
            log.warning(
                "No honest INRS on chain — will use wrong-model until a "
                "commitment is observed"
            )

        current = start_block
        while True:
            try:
                best = int(self.client.best_block()["number"])
                if best >= current:
                    messages = self.client.scan_blocks_for_messages(current, best)
                    for tx_id, raw in messages:
                        msg = decode_message(raw)
                        if isinstance(msg, InferenceResponse) and self._claimed_commitment is None:
                            self._extract_commitment(msg)
                        elif isinstance(msg, InferenceRequest):
                            self._handle_request(tx_id, msg)
                    current = best + 1
            except Exception as exc:
                log.warning("Poll error: %s", exc)
            time.sleep(self.poll_interval)


if __name__ == "__main__":
    from typing import Optional

    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(message)s")
    pk            = sys.argv[1] if len(sys.argv) > 1 else "35b5cc144faca7d7f220fca7ad3420090861d5231d80eb23e1013426847371c4"
    claimed_model = sys.argv[2] if len(sys.argv) > 2 else "Qwen/Qwen2.5-1.5B-Instruct"
    actual_model  = sys.argv[3] if len(sys.argv) > 3 else "Qwen/Qwen2.5-0.5B-Instruct"
    thor_url      = sys.argv[4] if len(sys.argv) > 4 else "http://localhost:8669"
    MaliciousInferenceNode(pk, claimed_model, actual_model, thor_url).run()
