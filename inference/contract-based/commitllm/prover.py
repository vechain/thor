"""
CommitLLM prover — contract-based version.

Key difference from the original prover: model_commitment is now the Merkle
root of the hook-layer weight matrix rows (W_root), not a sha256 sample hash.
This allows the on-chain fraud proof to verify that a claimed W row is
consistent with the registered model commitment.
"""
from __future__ import annotations

import hashlib

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer

from .receipt import LayerTrace, Receipt
from .merkle import compute_w_root, compute_vector_root, encode_float32


def _get_device() -> str:
    if torch.backends.mps.is_available():
        return "mps"
    if torch.cuda.is_available():
        return "cuda"
    return "cpu"


class CommitLLMProver:
    """
    Runs inference and produces a Receipt with:
      - model_commitment: W_root = MerkleRoot(rows of hook-layer W)
      - layer_traces:     {layer_name: LayerTrace(x, z)}

    The receipt's x/z can be committed on-chain via x_root/z_root.
    """

    def __init__(self, model_name: str):
        self.model_name = model_name
        device = _get_device()
        self.model = AutoModelForCausalLM.from_pretrained(
            model_name, torch_dtype=torch.float16, device_map=device
        )
        self.model.eval()
        self.tokenizer = AutoTokenizer.from_pretrained(model_name)
        self._w_root: str | None = None
        self._hook_weight: torch.Tensor | None = None
        self._hook_layer_name: str | None = None

    @property
    def model_commitment(self) -> str:
        """W_root — Merkle root of hook layer weight matrix rows."""
        if self._w_root is None:
            W, name = self._get_hook_weight()
            self._hook_weight = W
            self._hook_layer_name = name
            self._w_root = compute_w_root(W)
        return self._w_root

    @property
    def hook_weight(self) -> torch.Tensor:
        """Hook layer weight matrix (needed for challenger's dot product)."""
        self.model_commitment  # ensure loaded
        return self._hook_weight

    @property
    def hook_layer_name(self) -> str:
        self.model_commitment
        return self._hook_layer_name

    def _get_hook_weight(self) -> tuple[torch.Tensor, str]:
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
        n = len(layers)
        name, module = layers[n // 2]
        return module.weight.detach().cpu(), name

    def _select_hook_layers(self) -> list[tuple[str, torch.nn.Module]]:
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
        n = len(layers)
        return [layers[n // 2]]

    def generate_with_proof(
        self, prompt: str, max_new_tokens: int = 100
    ) -> tuple[str, Receipt]:
        traces: dict[str, LayerTrace] = {}
        handles = []

        for layer_name, module in self._select_hook_layers():
            def make_hook(name: str):
                def hook_fn(mod, inp, out):
                    x = inp[0][0, -1, :].detach().cpu().to(torch.float32)
                    z = out[0, -1, :].detach().cpu().to(torch.float32)
                    traces[name] = LayerTrace(x=x.tolist(), z=z.tolist())
                return hook_fn
            handles.append(module.register_forward_hook(make_hook(layer_name)))

        try:
            inputs = self.tokenizer(prompt, return_tensors="pt").to(self.model.device)
            with torch.no_grad():
                output_ids = self.model.generate(
                    **inputs,
                    max_new_tokens=max_new_tokens,
                    do_sample=False,
                    temperature=1.0,
                    pad_token_id=self.tokenizer.eos_token_id,
                )
        finally:
            for h in handles:
                h.remove()

        new_ids = output_ids[0][inputs["input_ids"].shape[1]:]
        output_text = self.tokenizer.decode(new_ids, skip_special_tokens=True)

        receipt = Receipt(
            model_name=self.model_name,
            model_commitment=self.model_commitment,
            input_hash=hashlib.sha256(prompt.encode()).hexdigest(),
            output_hash=hashlib.sha256(output_text.encode()).hexdigest(),
            layer_traces=traces,
        )
        return output_text, receipt

    def compute_roots(self, trace: LayerTrace) -> tuple[str, str, bytes, bytes]:
        """
        Compute x_root, z_root and binary-encoded x/z for a layer trace.

        Returns: (x_root_hex, z_root_hex, x_bytes, z_bytes)
        """
        x_root = compute_vector_root(trace.x)
        z_root = compute_vector_root(trace.z)
        x_bytes = encode_float32(trace.x)
        z_bytes = encode_float32(trace.z)
        return x_root, z_root, x_bytes, z_bytes
