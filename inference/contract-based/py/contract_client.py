"""
ContractClient — VeChain InferenceMarketplace interface.

Reads (eth_call, event logs): VeChain's native REST API via ThorClient.
Writes (transactions): ThorClient.send_transaction (signed VeChain RLP tx).

The split is necessary because VeChain's chain ID is a 256-bit genesis block ID
which Hardhat / web3.py cannot sign with. All write transactions go through the
custom VeChain signer in vechain_client.py.

ABI encoding/decoding uses eth_abi so no web3.py dependency is needed.
"""
from __future__ import annotations

import json
import os
import struct
from typing import Any, Optional

import requests
from eth_abi import encode as abi_encode, decode as abi_decode
from eth_hash.auto import keccak

from vechain_client import ThorClient


# ── ABI helpers ───────────────────────────────────────────────────────────── #

def _selector(sig: str) -> bytes:
    """4-byte function selector from signature string."""
    return keccak(sig.encode())[:4]


def _event_topic(sig: str) -> str:
    """32-byte event topic (0x-prefixed hex) from signature string."""
    return "0x" + keccak(sig.encode()).hex()


def _encode_call(sig: str, types: list[str], values: list) -> bytes:
    sel = _selector(sig)
    if types:
        return sel + abi_encode(types, values)
    return sel


def _decode_return(types: list[str], data: str) -> tuple:
    raw = bytes.fromhex(data[2:] if data.startswith("0x") else data)
    return abi_decode(types, raw)


# ── Deployment loading ────────────────────────────────────────────────────── #

_DEPLOYMENT_PATH = os.path.join(os.path.dirname(__file__), "..", "deployment.json")

_deployment: dict | None = None


def _load_deployment() -> dict:
    global _deployment
    if _deployment is None:
        path = os.path.abspath(_DEPLOYMENT_PATH)
        with open(path) as f:
            _deployment = json.load(f)
    return _deployment


def get_contract_address() -> str:
    return _load_deployment()["address"]


# ── ContractClient ────────────────────────────────────────────────────────── #

class ContractClient:
    """
    High-level interface to InferenceMarketplace.sol.

    All write methods return a VeChain transaction ID (hex string).
    All read methods return decoded Python values.
    Event reads return a list of decoded log dicts.
    """

    def __init__(self, thor_url: str = "http://localhost:8669"):
        self.thor = ThorClient(thor_url)
        self._address: str | None = None

    @property
    def address(self) -> str:
        if self._address is None:
            self._address = get_contract_address()
        return self._address

    # ── Contract calls (read-only) ─────────────────────────────────────────── #

    def _call(self, sig: str, types_in: list[str], values_in: list,
              types_out: list[str]) -> tuple:
        data = _encode_call(sig, types_in, values_in)
        # VeChain simulation endpoint: POST /accounts/* with clauses array
        results = self.thor._post(
            "/accounts/*",
            {"clauses": [{"to": self.address, "value": "0x0", "data": "0x" + data.hex()}],
             "gas": 100_000},
        )
        result = results[0]
        if result.get("reverted"):
            raise RuntimeError(f"call reverted: {result.get('vmError')}")
        return _decode_return(types_out, result["data"])

    def get_model(self, model: str) -> tuple[str, bool]:
        """Returns (wRoot_hex, registered)."""
        wRoot, registered = self._call(
            "models(string)", ["string"], [model], ["bytes32", "bool"]
        )
        return "0x" + wRoot.hex(), registered

    def get_inference_node(self, address: str) -> tuple[int, bool]:
        """Returns (stake_wei, active)."""
        return self._call(
            "inferenceNodes(address)", ["address"], [address], ["uint256", "bool"]
        )

    def get_checker_node(self, address: str) -> tuple[int, bool]:
        """Returns (stake_wei, active)."""
        return self._call(
            "checkerNodes(address)", ["address"], [address], ["uint256", "bool"]
        )

    def get_request(self, request_id: bytes) -> dict:
        result = self._call(
            "getRequest(bytes32)", ["bytes32"], [request_id],
            ["address", "string", "bytes32", "uint32", "uint256", "uint256", "bool", "bool"],
        )
        return {
            "requester": result[0], "model": result[1], "inputHash": "0x" + result[2].hex(),
            "maxNewTokens": result[3], "payment": result[4], "blockNumber": result[5],
            "responded": result[6], "finalized": result[7],
        }

    def get_response(self, request_id: bytes) -> dict:
        result = self._call(
            "getResponse(bytes32)", ["bytes32"], [request_id],
            ["address", "bytes32", "bytes32", "bytes32", "bytes32", "uint256", "bool", "uint256", "uint256"],
        )
        return {
            "prover": result[0], "outputHash": "0x" + result[1].hex(),
            "modelCommitment": "0x" + result[2].hex(), "xRoot": "0x" + result[3].hex(),
            "zRoot": "0x" + result[4].hex(), "blockNumber": result[5],
            "slashed": result[6], "fraudVotes": result[7], "validVotes": result[8],
        }

    def challenge_window(self) -> int:
        r, = self._call("CHALLENGE_WINDOW()", [], [], ["uint256"])
        return r

    # ── Transactions (write) ───────────────────────────────────────────────── #

    def register_model(self, pk: str, model: str, w_root: str) -> str:
        """Register a model. w_root = '0x' + 64 hex chars."""
        data = _encode_call(
            "registerModel(string,bytes32)",
            ["string", "bytes32"],
            [model, bytes.fromhex(w_root[2:])],
        )
        return self.thor.send_transaction(pk, [(self.address, 0, data)], gas=200_000)

    def register_inference_node(self, pk: str, stake_wei: int) -> str:
        data = _encode_call("registerInferenceNode()", [], [])
        return self.thor.send_transaction(pk, [(self.address, stake_wei, data)], gas=200_000)

    def register_checker_node(self, pk: str, stake_wei: int) -> str:
        data = _encode_call("registerCheckerNode()", [], [])
        return self.thor.send_transaction(pk, [(self.address, stake_wei, data)], gas=200_000)

    def submit_request(
        self, pk: str, model: str, input_hash: bytes, user_pubkey: bytes,
        max_new_tokens: int, payment_wei: int = 0,
    ) -> tuple[str, bytes]:
        """
        Submit an inference request.
        Returns (tx_id, request_id_bytes).
        request_id is computed off-chain to match the contract's keccak256.
        """
        from eth_keys import keys as eth_keys
        import time

        sender = eth_keys.PrivateKey(bytes.fromhex(pk)).public_key.to_address()
        # Match contract: keccak256(sender, model, inputHash, blockNumber, timestamp)
        # We can't predict blockNumber/timestamp exactly, so we read requestId from event
        data = _encode_call(
            "submitRequest(string,bytes32,bytes32,uint32)",
            ["string", "bytes32", "bytes32", "uint32"],
            [model, input_hash, user_pubkey, max_new_tokens],
        )
        tx_id = self.thor.send_transaction(pk, [(self.address, payment_wei, data)], gas=200_000)
        return tx_id

    def submit_response(
        self, pk: str, request_id: bytes,
        output_hash: bytes, model_commitment: str,
        x_root: str, z_root: str,
        x_encoded: bytes, z_encoded: bytes,
    ) -> str:
        data = _encode_call(
            "submitResponse(bytes32,bytes32,bytes32,bytes32,bytes32,bytes,bytes)",
            ["bytes32", "bytes32", "bytes32", "bytes32", "bytes32", "bytes", "bytes"],
            [
                request_id,
                output_hash,
                bytes.fromhex(model_commitment[2:]),
                bytes.fromhex(x_root[2:]),
                bytes.fromhex(z_root[2:]),
                x_encoded,
                z_encoded,
            ],
        )
        return self.thor.send_transaction(pk, [(self.address, 0, data)], gas=800_000)

    def submit_verdict(self, pk: str, request_id: bytes, valid: bool, reason: str) -> str:
        data = _encode_call(
            "submitVerdict(bytes32,bool,string)",
            ["bytes32", "bool", "string"],
            [request_id, valid, reason],
        )
        return self.thor.send_transaction(pk, [(self.address, 0, data)], gas=200_000)

    def submit_fraud_proof(
        self, pk: str, request_id: bytes, disputed_index: int,
        w_row_bytes: bytes,
        w_merkle_path: list[str], x_merkle_path: list[str], z_merkle_path: list[str],
        x_i: int, z_i: int,
        bond_wei: int = 10**18,
    ) -> str:
        w_path = [bytes.fromhex(h[2:]) for h in w_merkle_path]
        x_path = [bytes.fromhex(h[2:]) for h in x_merkle_path]
        z_path = [bytes.fromhex(h[2:]) for h in z_merkle_path]
        data = _encode_call(
            "submitFraudProof(bytes32,uint256,bytes,bytes32[],bytes32[],bytes32[],int256,int256)",
            ["bytes32", "uint256", "bytes", "bytes32[]", "bytes32[]", "bytes32[]", "int256", "int256"],
            [request_id, disputed_index, w_row_bytes, w_path, x_path, z_path, x_i, z_i],
        )
        return self.thor.send_transaction(pk, [(self.address, bond_wei, data)], gas=2_000_000)

    def finalize_response(self, pk: str, request_id: bytes) -> str:
        data = _encode_call("finalizeResponse(bytes32)", ["bytes32"], [request_id])
        return self.thor.send_transaction(pk, [(self.address, 0, data)], gas=200_000)

    # ── Event log reads ────────────────────────────────────────────────────── #

    def get_events(
        self, event_sig: str, from_block: int = 0, to_block: int = 2**32 - 1,
    ) -> list[dict]:
        """
        Query event logs from the contract.
        Returns list of raw log dicts with 'topics', 'data', 'meta'.
        """
        topic0 = _event_topic(event_sig)
        body = {
            "range":       {"unit": "block", "from": from_block, "to": to_block},
            "options":     {"offset": 0, "limit": 256},
            "criteriaSet": [{"address": self.address.lower(), "topic0": topic0}],
            "order":       "asc",
        }
        return self.thor._post("/logs/event", body)

    def parse_inference_responded(self, log: dict) -> dict:
        """Decode InferenceResponded event log."""
        # topics[1] = requestId, topics[2] = prover (indexed)
        request_id = log["topics"][1]
        prover      = "0x" + log["topics"][2][-40:]
        raw = bytes.fromhex(log["data"][2:])
        # Non-indexed: outputHash(32), modelCommitment(32), xRoot(32), zRoot(32), xEncoded(bytes), zEncoded(bytes)
        (output_hash, model_commitment, x_root, z_root, x_enc, z_enc) = abi_decode(
            ["bytes32", "bytes32", "bytes32", "bytes32", "bytes", "bytes"], raw
        )
        return {
            "requestId":       request_id,
            "prover":          prover,
            "outputHash":      "0x" + output_hash.hex(),
            "modelCommitment": "0x" + model_commitment.hex(),
            "xRoot":           "0x" + x_root.hex(),
            "zRoot":           "0x" + z_root.hex(),
            "xEncoded":        x_enc,
            "zEncoded":        z_enc,
            "blockNumber":     log["meta"]["blockNumber"],
            "txId":            log["meta"]["txID"],
        }

    def parse_inference_requested(self, log: dict) -> dict:
        """Decode InferenceRequested event log."""
        request_id = log["topics"][1]
        requester   = "0x" + log["topics"][2][-40:]
        raw = bytes.fromhex(log["data"][2:])
        (model, input_hash, user_pubkey, max_new_tokens, payment) = abi_decode(
            ["string", "bytes32", "bytes32", "uint32", "uint256"], raw
        )
        return {
            "requestId":    request_id,
            "requester":    requester,
            "model":        model,
            "inputHash":    "0x" + input_hash.hex(),
            "userPubkey":   "0x" + user_pubkey.hex(),
            "maxNewTokens": max_new_tokens,
            "payment":      payment,
            "blockNumber":  log["meta"]["blockNumber"],
            "txId":         log["meta"]["txID"],
        }

    def parse_verdict_submitted(self, log: dict) -> dict:
        """Decode VerdictSubmitted event log."""
        request_id = log["topics"][1]
        checker     = "0x" + log["topics"][2][-40:]
        raw = bytes.fromhex(log["data"][2:])
        (valid, reason) = abi_decode(["bool", "string"], raw)
        return {
            "requestId":   request_id,
            "checker":     checker,
            "valid":       valid,
            "reason":      reason,
            "blockNumber": log["meta"]["blockNumber"],
            "txId":        log["meta"]["txID"],
        }

    def wait_tx(self, tx_id: str, timeout: float = 60.0) -> dict:
        """Block until tx is included. Returns the transaction dict."""
        import time
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                tx = self.thor.get_transaction(tx_id)
                if tx.get("meta"):
                    return tx
            except Exception:
                pass
            time.sleep(2)
        raise TimeoutError(f"tx {tx_id} not confirmed after {timeout}s")

    def get_request_id_from_tx(self, tx_id: str) -> Optional[bytes]:
        """Read InferenceRequested event from a submitRequest tx to get requestId."""
        logs = self.get_events(
            "InferenceRequested(bytes32,address,string,bytes32,bytes32,uint32,uint256)",
        )
        for log in logs:
            if log.get("meta", {}).get("txID") == tx_id:
                return bytes.fromhex(log["topics"][1][2:])
        return None
