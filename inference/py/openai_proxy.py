"""
OpenAI-compatible proxy that routes chat completions through VeChain.

  GET  /v1/models               → list available models
  POST /v1/chat/completions     → post INFR tx, poll for INRS, return response

Environment:
  THOR_URL   VeChain node URL (default: http://localhost:8669)
"""

from __future__ import annotations

import os
import sys
import time
import uuid

sys.path.insert(0, os.path.dirname(__file__))

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from protocol import (
    INFERENCE_ADDR,
    InferenceRequest,
    InferenceResponse,
    decode_message,
    encode_request,
)
from vechain_client import ThorClient

THOR_URL = os.getenv("THOR_URL", "http://localhost:8669")

MODEL_ID = "vechain-qwen2.5-1.5B-Instruct"
MODEL_HF_NAME = "Qwen/Qwen2.5-1.5B-Instruct"

# Dev "user" account — funded in solo genesis
USER_PRIVATE_KEY = "99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36"

POLL_TIMEOUT = 300   # seconds to wait for inference response
POLL_INTERVAL = 5    # seconds between block scans

app = FastAPI(title="VeChain OpenAI Proxy")
client = ThorClient(THOR_URL)


# --------------------------------------------------------------------------- #
# Request / response models                                                    #
# --------------------------------------------------------------------------- #

class Message(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    model: str
    messages: list[Message]
    max_tokens: int = 200
    stream: bool = False


# --------------------------------------------------------------------------- #
# Routes                                                                       #
# --------------------------------------------------------------------------- #

@app.get("/v1/models")
def list_models():
    return {
        "object": "list",
        "data": [
            {
                "id": MODEL_ID,
                "object": "model",
                "created": 1_700_000_000,
                "owned_by": "vechain",
            }
        ],
    }


@app.post("/v1/chat/completions")
def chat_completions(req: ChatRequest):
    user_msgs = [m for m in req.messages if m.role == "user"]
    if not user_msgs:
        raise HTTPException(status_code=400, detail="No user message in request")
    prompt = user_msgs[-1].content

    # Record best block BEFORE sending so we can't miss the INRS reply
    scan_from = int(client.best_block()["number"])

    infr = InferenceRequest.new(
        model=MODEL_HF_NAME,
        prompt=prompt,
        max_new_tokens=req.max_tokens,
    )
    raw = encode_request(infr)
    client.send_transaction(
        USER_PRIVATE_KEY,
        clauses=[(INFERENCE_ADDR, 0, raw)],
    )

    # Poll VeChain blocks until the inference node posts a matching INRS
    deadline = time.time() + POLL_TIMEOUT
    while time.time() < deadline:
        best = int(client.best_block()["number"])
        if best >= scan_from:
            for _, raw_msg in client.scan_blocks_for_messages(scan_from, best):
                msg = decode_message(raw_msg)
                if (
                    isinstance(msg, InferenceResponse)
                    and msg.request_id == infr.request_id
                ):
                    return _openai_response(req.model, msg.output)
            scan_from = best + 1
        time.sleep(POLL_INTERVAL)

    raise HTTPException(status_code=504, detail="Inference timed out")


# --------------------------------------------------------------------------- #
# Helpers                                                                      #
# --------------------------------------------------------------------------- #

def _openai_response(model: str, content: str) -> dict:
    return {
        "id": f"chatcmpl-{uuid.uuid4().hex[:12]}",
        "object": "chat.completion",
        "created": int(time.time()),
        "model": model,
        "choices": [
            {
                "index": 0,
                "message": {"role": "assistant", "content": content},
                "finish_reason": "stop",
            }
        ],
        "usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
    }
