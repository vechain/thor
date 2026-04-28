"""
OZ-compatible Merkle tree for CommitLLM commitments.

Leaf conventions (double-keccak256, matches InferenceMarketplace.sol):
  W row i :  keccak256(keccak256(abi.encodePacked(uint256(i), rowBytes)))
             rowBytes = float32 numpy tobytes() (little-endian)
  x[i]/z[i]: keccak256(keccak256(abi.encodePacked(uint256(i), int256(scaled))))
             scaled = round(value * SCALE)

Internal node: keccak256(min(left, right) + max(left, right))  — sorted pair
"""
from __future__ import annotations

import struct
from typing import Union

import numpy as np
from eth_hash.auto import keccak

SCALE = 1_000_000  # fixed-point scaling for activation elements


# ── Low-level helpers ─────────────────────────────────────────────────────── #

def _uint256(n: int) -> bytes:
    return n.to_bytes(32, "big")


def _int256(v: int) -> bytes:
    if v < 0:
        v = v + (1 << 256)
    return v.to_bytes(32, "big")


def _leaf_row(index: int, row_bytes: bytes) -> bytes:
    """Leaf for W row: keccak256(keccak256(uint256(i) ++ rowBytes))"""
    inner = keccak(_uint256(index) + row_bytes)
    return keccak(inner)


def _leaf_element(index: int, scaled_value: int) -> bytes:
    """Leaf for x/z element: keccak256(keccak256(uint256(i) ++ int256(v)))"""
    inner = keccak(_uint256(index) + _int256(scaled_value))
    return keccak(inner)


def _internal_node(left: bytes, right: bytes) -> bytes:
    """Internal node: keccak256(min(left, right) ++ max(left, right))"""
    a, b = (left, right) if left <= right else (right, left)
    return keccak(a + b)


# ── Tree building ─────────────────────────────────────────────────────────── #

def _build_tree(leaves: list[bytes]) -> list[list[bytes]]:
    """
    Build a complete binary Merkle tree from leaves.
    Pads with duplicate last leaf to reach next power of 2.
    Returns list of levels: [leaf_level, ..., root_level].
    """
    if not leaves:
        raise ValueError("empty leaf list")

    n = 1
    while n < len(leaves):
        n <<= 1
    padded = leaves + [leaves[-1]] * (n - len(leaves))

    levels = [padded]
    current = padded
    while len(current) > 1:
        nxt = []
        for i in range(0, len(current), 2):
            nxt.append(_internal_node(current[i], current[i + 1]))
        levels.append(nxt)
        current = nxt
    return levels


def _get_proof(levels: list[list[bytes]], leaf_index: int) -> list[bytes]:
    """Return sibling hashes from leaf to root (Merkle proof path)."""
    proof = []
    idx = leaf_index
    for level in levels[:-1]:  # exclude root level
        sibling = idx ^ 1
        if sibling < len(level):
            proof.append(level[sibling])
        else:
            proof.append(level[idx])
        idx >>= 1
    return proof


# ── W matrix (model weight) tree ─────────────────────────────────────────── #

def compute_w_root(W_tensor) -> str:
    """
    Compute W_root = MerkleRoot(rows of W).

    W_tensor: torch.Tensor [out_features, in_features], any float dtype.
    Returns '0x' + 64-char hex string.
    """
    W = _to_float32_numpy(W_tensor)
    leaves = [_leaf_row(i, W[i].tobytes()) for i in range(W.shape[0])]
    tree = _build_tree(leaves)
    return "0x" + tree[-1][0].hex()


def get_w_row_proof(W_tensor, row_index: int) -> tuple[bytes, list[str]]:
    """
    Get the float32 row bytes and Merkle proof for row_index.

    Returns:
      (row_bytes, proof_hex_list)
      row_bytes: float32 little-endian binary for the row
      proof_hex_list: ['0x...', ...] Merkle siblings from leaf to root
    """
    W = _to_float32_numpy(W_tensor)
    leaves = [_leaf_row(i, W[i].tobytes()) for i in range(W.shape[0])]
    tree = _build_tree(leaves)
    proof = _get_proof(tree, row_index)
    return W[row_index].tobytes(), ["0x" + h.hex() for h in proof]


# ── Activation vector (x / z) tree ────────────────────────────────────────── #

def compute_vector_root(values: list[float]) -> str:
    """
    Compute Merkle root for an activation vector (x or z).

    Values are scaled by SCALE and rounded to int256.
    Returns '0x' + 64-char hex string.
    """
    leaves = [_leaf_element(i, int(round(v * SCALE))) for i, v in enumerate(values)]
    tree = _build_tree(leaves)
    return "0x" + tree[-1][0].hex()


def get_element_proof(values: list[float], index: int) -> tuple[int, list[str]]:
    """
    Get the scaled int256 value and Merkle proof for values[index].

    Returns:
      (scaled_value, proof_hex_list)
      scaled_value: int, = round(values[index] * SCALE)
      proof_hex_list: ['0x...', ...] Merkle siblings from leaf to root
    """
    scaled = [int(round(v * SCALE)) for v in values]
    leaves = [_leaf_element(i, v) for i, v in enumerate(scaled)]
    tree = _build_tree(leaves)
    proof = _get_proof(tree, index)
    return scaled[index], ["0x" + h.hex() for h in proof]


# ── Encoding helpers ──────────────────────────────────────────────────────── #

def encode_float32(values: list[float]) -> bytes:
    """Pack a list of floats as float32 little-endian binary."""
    return struct.pack(f"<{len(values)}f", *values)


def decode_float32(data: bytes) -> list[float]:
    """Unpack float32 little-endian binary back to a list of floats."""
    n = len(data) // 4
    return list(struct.unpack(f"<{n}f", data[:n * 4]))


# ── Internal utility ──────────────────────────────────────────────────────── #

def _to_float32_numpy(tensor) -> np.ndarray:
    """Convert a torch or numpy tensor to a float32 numpy array."""
    try:
        import torch
        if isinstance(tensor, torch.Tensor):
            return tensor.detach().cpu().to(torch.float32).numpy()
    except ImportError:
        pass
    arr = np.asarray(tensor)
    if arr.dtype != np.float32:
        arr = arr.astype(np.float32)
    return arr
