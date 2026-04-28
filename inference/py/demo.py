"""
End-to-end demo of the VeChain inference PoC.

Runs the full cycle without daemons:
  1. User posts INFR tx
  2. Honest inference node processes it → INRS tx
  3. Checker verifies → INCH tx (verdict: VALID)
  4. Fraud demo: inference node uses cheaper model → INCH tx (verdict: FRAUD)

Usage:
    python demo.py [thor_url]        # default: http://localhost:8669

Prerequisites:
    - VeChain solo node running on thor_url
    - HuggingFace models cached (run integration tests first)
"""

from __future__ import annotations

import logging
import sys
import time

sys.path.insert(0, "/app")

from commitllm.prover import CommitLLMProver
from commitllm.receipt import Receipt
from commitllm.verifier import CommitLLMVerifier
from protocol import (
    INFERENCE_ADDR,
    InferenceChallenge,
    InferenceRequest,
    InferenceResponse,
    decode_message,
    encode_challenge,
    encode_request,
    encode_response,
)
from vechain_client import ThorClient

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s  %(levelname)-7s  %(message)s",
)
log = logging.getLogger("demo")

# Dev accounts from genesis/devnet.go
ACCOUNTS = {
    "user":      "99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36",
    "inference": "7b067f53d350f1cf20ec13df416b7b73e88a1dc7331bc904b92108b1e76a08b1",
    "checker":   "f4a1a17039216f535d42ec23732c79943ffb45a089fbb78a14daad0dae93e991",
}

MODEL_HONEST = "Qwen/Qwen2.5-1.5B-Instruct"
MODEL_FRAUD  = "Qwen/Qwen2.5-0.5B-Instruct"


def banner(title: str) -> None:
    log.info("")
    log.info("=" * 60)
    log.info("  %s", title)
    log.info("=" * 60)


def wait_tx(client: ThorClient, tx_id: str, timeout: float = 30.0) -> dict:
    """Wait for a transaction to be included in a block."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            tx = client.get_transaction(tx_id)
            if tx.get("meta"):
                return tx
        except Exception:
            pass
        time.sleep(2)
    raise TimeoutError(f"tx {tx_id} not confirmed after {timeout}s")


def do_inference_cycle(
    client: ThorClient,
    prompt: str,
    prover: CommitLLMProver,
    verifier: CommitLLMVerifier,
    label: str,
) -> None:
    """Run one full request → response → challenge cycle synchronously."""
    banner(label)

    # ── Step 1: User sends InferenceRequest ─────────────────────────────────
    req = InferenceRequest.new(model=prover.model_name, prompt=prompt, max_new_tokens=50)
    raw_req = encode_request(req)
    log.info("Step 1  INFR  request_id=%s  model=%s", req.request_id[:8], req.model)
    req_tx_id = client.send_transaction(
        ACCOUNTS["user"],
        clauses=[(INFERENCE_ADDR, 0, raw_req)],
    )
    log.info("        tx → %s", req_tx_id)
    wait_tx(client, req_tx_id)

    # ── Step 2: Inference node runs model ────────────────────────────────────
    log.info("Step 2  Running inference …")
    output, receipt = prover.generate_with_proof(prompt, max_new_tokens=50)
    log.info("        Output: %r", output[:100])
    log.info("        Commitment: %s…", receipt.model_commitment[:24])

    resp = InferenceResponse(
        request_id=req.request_id,
        request_tx=req_tx_id,
        model=prover.model_name,
        output=output,
        receipt_json=receipt.to_json(),
    )
    raw_resp = encode_response(resp)
    log.info("        INRS payload size: %d bytes", len(raw_resp))
    resp_tx_id = client.send_transaction(
        ACCOUNTS["inference"],
        clauses=[(INFERENCE_ADDR, 0, raw_resp)],
    )
    log.info("        tx → %s", resp_tx_id)
    wait_tx(client, resp_tx_id)

    # ── Step 3: Checker verifies ─────────────────────────────────────────────
    log.info("Step 3  Verifying receipt …")
    ok, reason = verifier.verify(receipt)
    verdict = "VALID ✓" if ok else "FRAUD ✗"
    log.info("        Verdict: %s  (%s)", verdict, reason)

    challenge = InferenceChallenge(
        response_tx=resp_tx_id,
        valid=ok,
        reason=reason,
    )
    raw_ch = encode_challenge(challenge)
    ch_tx_id = client.send_transaction(
        ACCOUNTS["checker"],
        clauses=[(INFERENCE_ADDR, 0, raw_ch)],
    )
    log.info("        INCH tx → %s", ch_tx_id)
    wait_tx(client, ch_tx_id)

    log.info("")
    log.info("  ► Result: %s", verdict)


def main() -> None:
    thor_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8669"
    client = ThorClient(thor_url)

    log.info("Connecting to VeChain node at %s …", thor_url)
    best = client.best_block()
    log.info("Chain tag: 0x%02x   Best block: #%s", client.chain_tag(), best["number"])

    PROMPT = "In one sentence, what is the capital of France?"

    log.info("Loading models …")
    prover_honest = CommitLLMProver(MODEL_HONEST)
    prover_fraud  = CommitLLMProver(MODEL_FRAUD)
    verifier      = CommitLLMVerifier(MODEL_HONEST)
    log.info("Models ready.")

    # ── Happy path ────────────────────────────────────────────────────────────
    do_inference_cycle(
        client,
        PROMPT,
        prover=prover_honest,
        verifier=verifier,
        label="HAPPY PATH — honest inference (1.5B model)",
    )

    # ── Fraud path ────────────────────────────────────────────────────────────
    # prover_fraud claims to be MODEL_HONEST but runs MODEL_FRAUD internally
    # The receipt will carry the 0.5B model's commitment, which verifier rejects.
    do_inference_cycle(
        client,
        PROMPT,
        prover=prover_fraud,
        verifier=verifier,
        label="FRAUD PATH — inference node runs cheap 0.5B, claims 1.5B",
    )

    banner("DEMO COMPLETE")


if __name__ == "__main__":
    main()
