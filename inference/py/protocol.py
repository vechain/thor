"""
VeChain inference protocol — encodes/decodes inference messages carried in
clause Data fields.  Each message is:

    <4-byte ASCII magic> + <compact JSON payload>

Three message types:
  INFR – InferenceRequest  (user → inference node)
  INRS – InferenceResponse (inference node → chain)
  INCH – InferenceChallenge (checker → chain)

Clause To address is always the well-known INFERENCE_ADDR so nodes can
filter by address instead of scanning every clause.
"""

from __future__ import annotations

import json
import uuid
from dataclasses import asdict, dataclass
from typing import Optional

# Magic prefixes (4 ASCII bytes)
MAGIC_REQUEST   = b"INFR"
MAGIC_RESPONSE  = b"INRS"
MAGIC_CHALLENGE = b"INCH"

# All inference messages are sent to this fixed address so nodes can filter
# by recipient rather than scanning every clause on every block.
INFERENCE_ADDR = "0x000000000000000000696e666572656e636541646472"[:42]


# --------------------------------------------------------------------------- #
# Message dataclasses                                                           #
# --------------------------------------------------------------------------- #

@dataclass
class InferenceRequest:
    request_id: str
    model: str
    prompt: str
    max_new_tokens: int = 100

    @staticmethod
    def new(model: str, prompt: str, max_new_tokens: int = 100) -> "InferenceRequest":
        return InferenceRequest(
            request_id=str(uuid.uuid4()),
            model=model,
            prompt=prompt,
            max_new_tokens=max_new_tokens,
        )


@dataclass
class InferenceResponse:
    request_id: str
    request_tx: str        # tx ID of the InferenceRequest
    model: str
    output: str
    receipt_json: str      # serialised commitllm.Receipt


@dataclass
class InferenceChallenge:
    response_tx: str       # tx ID of the InferenceResponse being judged
    valid: bool
    reason: str


# --------------------------------------------------------------------------- #
# Encode / decode                                                               #
# --------------------------------------------------------------------------- #

def encode_request(req: InferenceRequest) -> bytes:
    return MAGIC_REQUEST + json.dumps(asdict(req), separators=(",", ":")).encode()


def encode_response(resp: InferenceResponse) -> bytes:
    return MAGIC_RESPONSE + json.dumps(asdict(resp), separators=(",", ":")).encode()


def encode_challenge(ch: InferenceChallenge) -> bytes:
    return MAGIC_CHALLENGE + json.dumps(asdict(ch), separators=(",", ":")).encode()


def decode_message(data: bytes) -> Optional[InferenceRequest | InferenceResponse | InferenceChallenge]:
    if len(data) < 4:
        return None
    magic, payload = data[:4], data[4:]
    try:
        d = json.loads(payload)
    except Exception:
        return None
    if magic == MAGIC_REQUEST:
        return InferenceRequest(**d)
    if magic == MAGIC_RESPONSE:
        return InferenceResponse(**d)
    if magic == MAGIC_CHALLENGE:
        return InferenceChallenge(**d)
    return None


def data_hex(raw: bytes) -> str:
    return "0x" + raw.hex()
