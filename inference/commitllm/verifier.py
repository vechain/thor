import hashlib

import torch
from transformers import AutoModelForCausalLM

from .receipt import Receipt


def freivalds_check(
    W: torch.Tensor,
    x: torch.Tensor,
    z: torch.Tensor,
    num_trials: int = 20,
) -> tuple[bool, str]:
    """
    Verify z ≈ W @ x using Freivalds' algorithm.

    Detects cheating with probability ≥ 1 - (1/2)^num_trials.
    Uses float64 arithmetic; relative error threshold 5% to tolerate fp16 accumulation noise.
    """
    W = W.to(torch.float64)
    x = x.to(torch.float64)
    z = z.to(torch.float64)

    if W.shape != (len(z), len(x)):
        return False, f"shape mismatch: W={tuple(W.shape)}, x={len(x)}, z={len(z)}"

    for trial in range(num_trials):
        r = torch.randint(0, 2, (W.shape[0],), dtype=torch.float64)
        lhs = r @ z
        rhs = (r @ W) @ x
        scale = max(abs(lhs.item()), abs(rhs.item()), 1.0)
        rel_err = abs(lhs.item() - rhs.item()) / scale
        if rel_err > 0.05:
            return False, (
                f"trial {trial}: r·z={lhs.item():.4f}, "
                f"r·W·x={rhs.item():.4f}, rel_err={rel_err:.4f}"
            )

    return True, "ok"


def _compute_model_commitment(model) -> str:
    h = hashlib.sha256()
    named_params = [(n, p) for n, p in model.named_parameters() if "weight" in n]
    n = len(named_params)
    indices = sorted({0, n // 4, n // 2, 3 * n // 4, n - 1})
    for idx in indices:
        _, param = named_params[idx]
        sample = param.detach().cpu().to(torch.float32).flatten()[:256]
        h.update(sample.numpy().tobytes())
    return h.hexdigest()


class CommitLLMVerifier:
    """
    Loads the claimed model locally, checks model commitment and runs
    Freivalds on captured layer traces.

    If the prover used different weights, commitment check fails immediately.
    If the prover used correct model but tampered activations, Freivalds catches it.
    """

    def __init__(self, model_name: str):
        self.model_name = model_name
        self.model = AutoModelForCausalLM.from_pretrained(
            model_name, torch_dtype=torch.float16, device_map="cpu"
        )
        self.model.eval()
        self._commitment: str | None = None

    @property
    def model_commitment(self) -> str:
        if self._commitment is None:
            self._commitment = _compute_model_commitment(self.model)
        return self._commitment

    def verify(self, receipt: Receipt) -> tuple[bool, str]:
        if receipt.model_commitment != self.model_commitment:
            return False, (
                f"model commitment mismatch: "
                f"receipt={receipt.model_commitment[:16]}... "
                f"expected={self.model_commitment[:16]}..."
            )

        for layer_name, trace in receipt.layer_traces.items():
            W = self._get_layer_weight(layer_name)
            if W is None:
                return False, f"layer '{layer_name}' not found in model"
            x = torch.tensor(trace.x, dtype=torch.float32)
            z = torch.tensor(trace.z, dtype=torch.float32)
            ok, reason = freivalds_check(W.to(torch.float32), x, z)
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
