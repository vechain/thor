"""
Minimal VeChain HTTP client + legacy transaction signer.

Transaction encoding follows the VeChain Thor legacyTransaction RLP spec:
  [ChainTag, BlockRef, Expiration, Clauses, GasPriceCoef, Gas,
   DependsOn, Nonce, Reserved, Signature]

Signing hash = blake2b-256( RLP( signing_fields_without_signature ) )
"""

from __future__ import annotations

import hashlib
import random
import time
from typing import Optional

import requests
from eth_keys import keys as eth_keys


# --------------------------------------------------------------------------- #
# Minimal RLP encoder                                                           #
# --------------------------------------------------------------------------- #

def _rlp_length(n: int, offset: int) -> bytes:
    if n < 56:
        return bytes([n + offset])
    lb = n.to_bytes((n.bit_length() + 7) // 8, "big")
    return bytes([len(lb) + offset + 55]) + lb


def _rlp_bytes(b: bytes) -> bytes:
    if len(b) == 0:
        return b"\x80"
    if len(b) == 1 and b[0] < 0x80:
        return b
    return _rlp_length(len(b), 0x80) + b


def _rlp_int(n: int) -> bytes:
    """RLP-encode an unsigned integer (strips leading zeros, matches Go eth-RLP)."""
    if n == 0:
        return b"\x80"
    nb = n.to_bytes((n.bit_length() + 7) // 8, "big")
    return _rlp_bytes(nb)


def _rlp_list(items: list[bytes]) -> bytes:
    body = b"".join(items)
    return _rlp_length(len(body), 0xC0) + body


# --------------------------------------------------------------------------- #
# Transaction signing                                                           #
# --------------------------------------------------------------------------- #

def _encode_clause(to: Optional[str], value: int, data: bytes) -> bytes:
    to_enc = _rlp_bytes(bytes.fromhex(to[2:]) if to else b"")
    val_enc = _rlp_int(value)
    data_enc = _rlp_bytes(data)
    return _rlp_list([to_enc, val_enc, data_enc])


def build_and_sign_tx(
    chain_tag: int,
    block_ref: int,         # uint64 from first 8 bytes of best block ID
    private_key_hex: str,
    clauses: list[tuple[Optional[str], int, bytes]],  # (to, value, data)
    gas: int = 300_000,
    expiration: int = 720,
    nonce: Optional[int] = None,
) -> str:
    """Return hex-encoded signed raw transaction (0x prefixed)."""
    if nonce is None:
        nonce = random.getrandbits(64)

    clause_items = [_encode_clause(to, val, data) for to, val, data in clauses]

    # Signing fields (no signature yet)
    signing_fields = [
        _rlp_int(chain_tag),
        _rlp_int(block_ref),
        _rlp_int(expiration),
        _rlp_list(clause_items),
        _rlp_int(0),           # gasPriceCoef
        _rlp_int(gas),
        b"\x80",               # dependsOn = nil
        _rlp_int(nonce),
        b"\xc0",               # reserved = empty list
    ]
    signing_rlp = _rlp_list(signing_fields)

    # blake2b-256 signing hash
    h = hashlib.blake2b(signing_rlp, digest_size=32)
    signing_hash = h.digest()

    # secp256k1 signature (r + s + v, 65 bytes)
    pk = eth_keys.PrivateKey(bytes.fromhex(private_key_hex))
    sig = pk.sign_msg_hash(signing_hash)
    sig_bytes = sig.to_bytes()  # 65 bytes: r(32)+s(32)+v(1)

    # Full encoded transaction
    full_fields = signing_fields + [_rlp_bytes(sig_bytes)]
    raw = _rlp_list(full_fields)
    return "0x" + raw.hex()


# --------------------------------------------------------------------------- #
# HTTP client                                                                   #
# --------------------------------------------------------------------------- #

class ThorClient:
    def __init__(self, url: str = "http://localhost:8669"):
        self.url = url.rstrip("/")
        self._chain_tag: Optional[int] = None

    def _get(self, path: str) -> dict:
        r = requests.get(f"{self.url}{path}", timeout=10)
        r.raise_for_status()
        return r.json()

    def _post(self, path: str, body: dict) -> dict:
        r = requests.post(f"{self.url}{path}", json=body, timeout=30)
        if not r.ok:
            raise requests.HTTPError(
                f"{r.status_code} {r.reason}: {r.text[:300]}", response=r
            )
        return r.json()

    def best_block(self) -> dict:
        return self._get("/blocks/best")

    def get_block(self, revision: str = "best") -> dict:
        return self._get(f"/blocks/{revision}?expanded=true")

    def get_block_header(self, revision: str = "best") -> dict:
        """Compact block header without expanded transaction data (faster)."""
        return self._get(f"/blocks/{revision}")

    def get_transaction(self, tx_id: str) -> dict:
        return self._get(f"/transactions/{tx_id}")

    def chain_tag(self) -> int:
        if self._chain_tag is None:
            genesis = self._get("/blocks/0")
            genesis_id = genesis["id"]  # 0x + 64 hex chars
            self._chain_tag = int(genesis_id[-2:], 16)  # last byte
        return self._chain_tag

    def block_ref(self) -> int:
        """Return first 8 bytes of best block ID as uint64."""
        best_id = self.best_block()["id"]
        return int(best_id[2:18], 16)  # first 8 bytes = 16 hex chars

    def send_transaction(
        self,
        private_key_hex: str,
        clauses: list[tuple[Optional[str], int, bytes]],
        gas: Optional[int] = None,
    ) -> str:
        """Sign and broadcast a transaction. Returns tx ID.

        If gas is None, it is estimated from clause data sizes using VeChain's
        intrinsic gas rules (5000 base + 1500/clause + 68/non-zero byte + 4/zero byte).
        """
        if gas is None:
            # VeChain intrinsic gas:
            #   TxGas=5000  +  per clause: ClauseGas=16000 (=ETH TxGas 21000 - 5000)
            #   + TxDataNonZeroGas=68 per non-zero byte, TxDataZeroGas=4 per zero byte
            gas = 5000 + sum(
                16_000 + sum(68 if b != 0 else 4 for b in (data or b""))
                for (_, _, data) in clauses
            )

        # Hard cap: stay under the default solo/testnet block gas limit (40M)
        gas = min(gas, 39_000_000)

        raw = build_and_sign_tx(
            chain_tag=self.chain_tag(),
            block_ref=self.block_ref(),
            private_key_hex=private_key_hex,
            clauses=clauses,
            gas=gas,
        )
        result = self._post("/transactions", {"raw": raw})
        return result["id"]

    def scan_blocks_for_messages(
        self, from_block: int, to_block: int
    ) -> list[tuple[str, bytes]]:
        """
        Scan blocks [from_block, to_block] and return list of (tx_id, clause_data)
        for all clauses that start with a known magic prefix.
        """
        from protocol import MAGIC_REQUEST, MAGIC_RESPONSE, MAGIC_CHALLENGE

        known_magics = {MAGIC_REQUEST, MAGIC_RESPONSE, MAGIC_CHALLENGE}
        found = []
        for block_num in range(from_block, to_block + 1):
            try:
                block = self.get_block(str(block_num))
            except Exception:
                continue
            for tx in block.get("transactions") or []:
                tx_id = tx.get("id", "")
                for clause in tx.get("clauses") or []:
                    raw_data = clause.get("data", "0x")
                    if len(raw_data) < 10:
                        continue
                    b = bytes.fromhex(raw_data[2:])
                    if len(b) >= 4 and b[:4] in known_magics:
                        found.append((tx_id, b))
        return found

    def wait_for_block(self, target_block: int, timeout: float = 60.0) -> bool:
        """Poll until best block >= target_block. Returns True on success."""
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                best = int(self.best_block()["number"])
                if best >= target_block:
                    return True
            except Exception:
                pass
            time.sleep(2)
        return False
