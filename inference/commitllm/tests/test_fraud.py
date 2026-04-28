import copy
import pytest

pytestmark = pytest.mark.integration


def test_wrong_model_detected(prover_b, verifier_a):
    """Prover uses cheap 0.5B model; verifier expects 1.5B — commitment mismatch."""
    _, receipt = prover_b.generate_with_proof("What is 2 + 2?", max_new_tokens=20)
    ok, reason = verifier_a.verify(receipt)
    assert not ok, "wrong model should be rejected"
    assert "commitment mismatch" in reason


def test_tampered_z_detected(prover_a, verifier_a):
    """Prover generates honest receipt, then z activations are forged — Freivalds catches it."""
    _, receipt = prover_a.generate_with_proof("Hello world", max_new_tokens=20)

    first_layer = next(iter(receipt.layer_traces))
    trace = receipt.layer_traces[first_layer]
    # Mutate z significantly so Freivalds fails
    tampered_z = [v + 100.0 for v in trace.z]

    from commitllm.receipt import LayerTrace

    receipt.layer_traces[first_layer] = LayerTrace(x=trace.x, z=tampered_z)

    ok, reason = verifier_a.verify(receipt)
    assert not ok, "tampered activations should be detected"
    assert "Freivalds" in reason


def test_wrong_model_commitment_differs(prover_a, prover_b):
    """The two models must produce different commitments."""
    assert prover_a.model_commitment != prover_b.model_commitment


def test_wrong_model_receipt_has_different_commitment(prover_a, prover_b):
    """Receipt from fraud prover carries a different commitment than the honest one."""
    _, receipt_a = prover_a.generate_with_proof("Hi", max_new_tokens=10)
    _, receipt_b = prover_b.generate_with_proof("Hi", max_new_tokens=10)
    assert receipt_a.model_commitment != receipt_b.model_commitment
