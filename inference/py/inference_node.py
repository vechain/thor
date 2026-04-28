"""
Inference node daemon.

Polls VeChain blocks for INFR (InferenceRequest) messages, runs CommitLLM
inference on the requested model, and posts an INRS (InferenceResponse) tx.
"""

from __future__ import annotations

import logging
import sys
import time

from protocol import (
    INFERENCE_ADDR,
    InferenceRequest,
    InferenceResponse,
    decode_message,
    encode_response,
    data_hex,
)
from vechain_client import ThorClient

sys.path.insert(0, "/app")  # commitllm package lives here in Docker

log = logging.getLogger("inference_node")


class InferenceNode:
    def __init__(
        self,
        private_key_hex: str,
        model_name: str,
        thor_url: str = "http://thor:8669",
        poll_interval: float = 5.0,
    ):
        self.pk = private_key_hex
        self.model_name = model_name
        self.client = ThorClient(thor_url)
        self.poll_interval = poll_interval
        self._prover = None
        self._seen_requests: set[str] = set()

    def _prover_instance(self):
        if self._prover is None:
            from commitllm.prover import CommitLLMProver

            log.info("Loading model %s …", self.model_name)
            self._prover = CommitLLMProver(self.model_name)
            log.info("Model loaded (commitment %s…)", self._prover.model_commitment[:16])
        return self._prover

    def _handle_request(self, tx_id: str, req: InferenceRequest) -> None:
        if req.request_id in self._seen_requests:
            return
        self._seen_requests.add(req.request_id)

        if req.model != self.model_name:
            log.warning("Request for %s but we run %s — skipping", req.model, self.model_name)
            return

        log.info("[%s] Running inference for prompt: %r", req.request_id[:8], req.prompt[:60])
        prover = self._prover_instance()
        output, receipt = prover.generate_with_proof(
            req.prompt, max_new_tokens=req.max_new_tokens
        )
        log.info("[%s] Output: %r", req.request_id[:8], output[:80])

        resp = InferenceResponse(
            request_id=req.request_id,
            request_tx=tx_id,
            model=self.model_name,
            output=output,
            receipt_json=receipt.to_json(),
        )
        raw_data = encode_response(resp)
        sent_tx = self.client.send_transaction(
            self.pk,
            clauses=[(INFERENCE_ADDR, 0, raw_data)],
        )
        log.info("[%s] Posted INRS tx %s", req.request_id[:8], sent_tx)

    def run(self, start_block: Optional[int] = None) -> None:
        log.info("Inference node started (model=%s)", self.model_name)
        if start_block is None:
            start_block = int(self.client.best_block()["number"])
        current = start_block

        while True:
            try:
                best = int(self.client.best_block()["number"])
                if best >= current:
                    messages = self.client.scan_blocks_for_messages(current, best)
                    for tx_id, raw in messages:
                        msg = decode_message(raw)
                        if isinstance(msg, InferenceRequest):
                            self._handle_request(tx_id, msg)
                    current = best + 1
            except Exception as exc:
                log.warning("Poll error: %s", exc)
            time.sleep(self.poll_interval)


# Allow running directly:
#   python inference_node.py <private_key> <model> [thor_url]
if __name__ == "__main__":
    from typing import Optional

    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(message)s")
    pk = sys.argv[1] if len(sys.argv) > 1 else "7b067f53d350f1cf20ec13df416b7b73e88a1dc7331bc904b92108b1e76a08b1"
    model = sys.argv[2] if len(sys.argv) > 2 else "Qwen/Qwen2.5-1.5B-Instruct"
    thor_url = sys.argv[3] if len(sys.argv) > 3 else "http://localhost:8669"
    InferenceNode(pk, model, thor_url).run()
