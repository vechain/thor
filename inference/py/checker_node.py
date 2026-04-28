"""
Checker node daemon.

Polls VeChain blocks for INRS (InferenceResponse) messages, verifies the
CommitLLM receipt (model commitment + Freivalds), and posts an INCH
(InferenceChallenge) tx with the verdict.
"""

from __future__ import annotations

import logging
import sys
import time
from typing import Optional

from protocol import (
    INFERENCE_ADDR,
    InferenceChallenge,
    InferenceResponse,
    decode_message,
    encode_challenge,
    data_hex,
)
from vechain_client import ThorClient

sys.path.insert(0, "/app")

log = logging.getLogger("checker_node")


class CheckerNode:
    def __init__(
        self,
        private_key_hex: str,
        thor_url: str = "http://thor:8669",
        poll_interval: float = 5.0,
    ):
        self.pk = private_key_hex
        self.client = ThorClient(thor_url)
        self.poll_interval = poll_interval
        self._verifiers: dict[str, object] = {}  # model_name → CommitLLMVerifier
        self._seen_responses: set[str] = set()

    def _get_verifier(self, model_name: str):
        if model_name not in self._verifiers:
            from commitllm.verifier import CommitLLMVerifier

            log.info("Loading verifier for %s …", model_name)
            self._verifiers[model_name] = CommitLLMVerifier(model_name)
            log.info("Verifier ready (commitment %s…)", self._verifiers[model_name].model_commitment[:16])
        return self._verifiers[model_name]

    def _handle_response(self, tx_id: str, resp: InferenceResponse) -> None:
        if tx_id in self._seen_responses:
            return
        self._seen_responses.add(tx_id)

        log.info("[%s] Checking response from tx %s …", resp.request_id[:8], tx_id)
        try:
            verifier = self._get_verifier(resp.model)
            from commitllm.receipt import Receipt

            receipt = Receipt.from_json(resp.receipt_json)
            ok, reason = verifier.verify(receipt)
        except Exception as exc:
            ok, reason = False, f"verification error: {exc}"

        verdict = "VALID" if ok else "FRAUD"
        log.info("[%s] Verdict: %s — %s", resp.request_id[:8], verdict, reason)

        challenge = InferenceChallenge(
            response_tx=tx_id,
            valid=ok,
            reason=reason,
        )
        raw_data = encode_challenge(challenge)
        sent_tx = self.client.send_transaction(
            self.pk,
            clauses=[(INFERENCE_ADDR, 0, raw_data)],
        )
        log.info("[%s] Posted INCH tx %s", resp.request_id[:8], sent_tx)

    def run(self, start_block: Optional[int] = None) -> None:
        log.info("Checker node started")
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
                        if isinstance(msg, InferenceResponse):
                            self._handle_response(tx_id, msg)
                    current = best + 1
            except Exception as exc:
                log.warning("Poll error: %s", exc)
            time.sleep(self.poll_interval)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(message)s")
    pk = sys.argv[1] if len(sys.argv) > 1 else "f4a1a17039216f535d42ec23732c79943ffb45a089fbb78a14daad0dae93e991"
    thor_url = sys.argv[2] if len(sys.argv) > 2 else "http://localhost:8669"
    CheckerNode(pk, thor_url).run()
