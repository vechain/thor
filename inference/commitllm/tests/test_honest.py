import hashlib
import pytest

pytestmark = pytest.mark.integration


def test_honest_inference_passes(prover_a, verifier_a):
    _, receipt = prover_a.generate_with_proof("What is 2 + 2?", max_new_tokens=50)
    ok, reason = verifier_a.verify(receipt)
    assert ok, f"honest inference failed verification: {reason}"


def test_receipt_fields_populated(prover_a):
    _, receipt = prover_a.generate_with_proof("Hello", max_new_tokens=20)
    assert len(receipt.model_commitment) == 64
    assert len(receipt.input_hash) == 64
    assert len(receipt.output_hash) == 64
    assert len(receipt.layer_traces) >= 1, (
        f"expected at least 1 hook layer, got {len(receipt.layer_traces)}"
    )
    for name, trace in receipt.layer_traces.items():
        assert len(trace.x) > 0
        assert len(trace.z) > 0


def test_output_hash_matches(prover_a):
    output_text, receipt = prover_a.generate_with_proof(
        "Name one planet.", max_new_tokens=30
    )
    expected = hashlib.sha256(output_text.encode()).hexdigest()
    assert receipt.output_hash == expected


def test_receipt_json_roundtrip_with_real_model(prover_a, verifier_a):
    from commitllm.receipt import Receipt

    _, receipt = prover_a.generate_with_proof("Say hi.", max_new_tokens=20)
    receipt2 = Receipt.from_json(receipt.to_json())
    ok, reason = verifier_a.verify(receipt2)
    assert ok, f"verify after JSON roundtrip failed: {reason}"
