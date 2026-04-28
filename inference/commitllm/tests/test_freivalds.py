import torch
import pytest

from commitllm.verifier import freivalds_check


def test_honest_matmul_small():
    torch.manual_seed(0)
    W = torch.randn(8, 6, dtype=torch.float32)
    x = torch.randn(6, dtype=torch.float32)
    z = W @ x
    ok, reason = freivalds_check(W, x, z)
    assert ok, reason


def test_honest_matmul_large():
    torch.manual_seed(1)
    W = torch.randn(64, 32, dtype=torch.float32)
    x = torch.randn(32, dtype=torch.float32)
    z = W @ x
    ok, reason = freivalds_check(W, x, z)
    assert ok, reason


def test_tampered_z_detected():
    torch.manual_seed(2)
    W = torch.randn(16, 12, dtype=torch.float32)
    x = torch.randn(12, dtype=torch.float32)
    z = W @ x
    z_tampered = z.clone()
    z_tampered[0] += 100.0
    ok, reason = freivalds_check(W, x, z_tampered, num_trials=20)
    assert not ok, "tampered z should be detected"


def test_wrong_weights_detected():
    torch.manual_seed(3)
    W_true = torch.randn(16, 12, dtype=torch.float32)
    W_wrong = torch.randn(16, 12, dtype=torch.float32)
    x = torch.randn(12, dtype=torch.float32)
    z = W_true @ x
    ok, reason = freivalds_check(W_wrong, x, z, num_trials=20)
    assert not ok, "wrong weights should be detected"


def test_false_positive_rate():
    # Honest computation should never be flagged across many runs
    torch.manual_seed(42)
    false_positives = 0
    for _ in range(100):
        W = torch.randn(32, 24, dtype=torch.float32)
        x = torch.randn(24, dtype=torch.float32)
        z = W @ x
        ok, _ = freivalds_check(W, x, z, num_trials=10)
        if not ok:
            false_positives += 1
    assert false_positives == 0, f"{false_positives} false positives in 100 runs"


def test_shape_mismatch_rejected():
    W = torch.randn(8, 6)
    x = torch.randn(10)  # wrong size
    z = torch.randn(8)
    ok, reason = freivalds_check(W, x, z)
    assert not ok
    assert "shape mismatch" in reason


def test_fp16_noise_tolerated():
    # FP16 activations introduce small rounding errors; verifier uses fp32 → should pass
    torch.manual_seed(5)
    W = torch.randn(32, 32, dtype=torch.float16)
    x = torch.randn(32, dtype=torch.float16)
    z = (W.to(torch.float32) @ x.to(torch.float32))
    ok, reason = freivalds_check(W.to(torch.float32), x.to(torch.float32), z)
    assert ok, reason
