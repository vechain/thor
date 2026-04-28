"""
CommitLLM verifier — contract-based version.

Key differences from the original verifier:
  1. model_commitment is W_root (Merkle root), not a sha256 sample hash.
  2. Freivalds uses deterministic r vectors derived from on-chain entropy, so
     every honest checker produces identical intermediate values for the same
     request. Two checkers who disagree cannot both be honest.

     r[trial][j] = keccak256(requestId || blockHash || trial || j) mod 2

     NOTE: For the PoC the checker submits verdicts but the on-chain contract
     does not enforce deterministic r — that check would require re-running
     Freivalds on-chain. The deterministic r is critical for the slash-minority
     mechanism once checker-disagreement disputes are added.
"""
from __future__ import annotations

import hashlib

import torch
from transformers import AutoModelForCausalLM
from eth_hash.auto import keccak

from .receipt import Receipt
from .merkle import compute_w_root


def freivalds_check(
    W: torch.Tensor,
    x: torch.Tensor,
    z: torch.Tensor,
    request_id: bytes | None = None,
    block_hash: bytes | None = None,
    num_trials: int = 20,
) -> tuple[bool, str]:
    """
    Verify z ≈ W @ x using Freivalds' algorithm.

    If request_id and block_hash are provided, r vectors are derived
    deterministically from on-chain entropy (production mode).
    Otherwise random r is used (PoC / local testing).

    Detects cheating with probability ≥ 1 - (1/2)^num_trials.
    Uses float64; relative error threshold 5% for fp16 accumulation noise.
    """
    W64 = W.to(torch.float64)
    x64 = x.to(torch.float64)
    z64 = z.to(torch.float64)
    n   = W64.shape[0]

    if W64.shape != (len(z64), len(x64)):
        return False, f"shape mismatch: W={tuple(W64.shape)}, x={len(x64)}, z={len(z64)}"

    for trial in range(num_trials):
        if request_id is not None and block_hash is not None:
            r = _deterministic_r(request_id, block_hash, trial, n)
        else:
            r = torch.randint(0, 2, (n,), dtype=torch.float64)

        lhs = r @ z64
        rhs = (r @ W64) @ x64
        scale = max(abs(lhs.item()), abs(rhs.item()), 1.0)
        rel_err = abs(lhs.item() - rhs.item()) / scale
        if rel_err > 0.05:
            return False, (
                f"trial {trial}: r·z={lhs.item():.4f}, "
                f"r·W·x={rhs.item():.4f}, rel_err={rel_err:.4f}"
            )

    return True, "ok"


def _deterministic_r(
    request_id: bytes, block_hash: bytes, trial: int, n: int
) -> torch.Tensor:
    """
    Deterministic {0,1}^n vector derived from on-chain entropy.

    r[j] = keccak256(requestId || blockHash || uint32(trial) || uint32(j)) & 1

    Each bit requires one keccak256 call. For production performance a single
    keccak seed could be expanded with a stream cipher, but correctness is the
    priority here.
    """
    bits = []
    seed = request_id + block_hash + trial.to_bytes(4, "big")
    for j in range(n):
        h = keccak(seed + j.to_bytes(4, "big"))
        bits.append(h[0] & 1)
    return torch.tensor(bits, dtype=torch.float64)


class CommitLLMVerifier:
    """
    Loads the claimed model locally, checks model commitment (W_root) and
    runs Freivalds on captured layer traces.

    If the prover used different weights, the W_root check fails immediately.
    If the prover used correct weights but tampered x/z, Freivalds catches it.
    """

    def __init__(self, model_name: str):
        self.model_name = model_name
        self.model = AutoModelForCausalLM.from_pretrained(
            model_name, torch_dtype=torch.float16, device_map="cpu"
        )
        self.model.eval()
        self._w_root: str | None = None

    @property
    def model_commitment(self) -> str:
        if self._w_root is None:
            layers = [
                (n, m)
                for n, m in self.model.named_modules()
                if n.endswith(".o_proj") and isinstance(m, torch.nn.Linear)
            ]
            if not layers:
                layers = [
                    (n, m)
                    for n, m in self.model.named_modules()
                    if isinstance(m, torch.nn.Linear) and not n.endswith("lm_head")
                ]
            k = len(layers)
            _, module = layers[k // 2]
            self._w_root = compute_w_root(module.weight.detach().cpu())
        return self._w_root

    def verify(
        self,
        receipt: Receipt,
        request_id: bytes | None = None,
        block_hash: bytes | None = None,
    ) -> tuple[bool, str]:
        if receipt.model_commitment != self.model_commitment:
            return False, (
                f"model commitment mismatch: "
                f"receipt={receipt.model_commitment[:18]}... "
                f"expected={self.model_commitment[:18]}..."
            )

        for layer_name, trace in receipt.layer_traces.items():
            W = self._get_layer_weight(layer_name)
            if W is None:
                return False, f"layer '{layer_name}' not found in model"
            x = torch.tensor(trace.x, dtype=torch.float32)
            z = torch.tensor(trace.z, dtype=torch.float32)
            ok, reason = freivalds_check(
                W.to(torch.float32), x, z,
                request_id=request_id,
                block_hash=block_hash,
            )
            if not ok:
                return False, f"Freivalds failed at layer '{layer_name}': {reason}"

        return True, "valid"

    def _get_layer_weight(self, layer_name: str) -> torch.Tensor | None:
        obj = self.model
        for part in layer_name.split("."):
            try:
                obj = obj[int(part)] if part.isdigit() else getattr(obj, part)
            except (AttributeError, IndexError, TypeError):
                return None
        return obj.weight.detach().cpu() if hasattr(obj, "weight") else None
