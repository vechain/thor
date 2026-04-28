# VeChain Inference PoC

A proof-of-concept for **verifiable AI inference on VeChain**. LLM providers post cryptographic receipts on-chain alongside their responses; independent checker nodes verify those receipts and flag fraud — all without re-running the model.

---

## Table of Contents

1. [Motivation](#motivation)
2. [How CommitLLM Works](#how-commitllm-works)
3. [On-Chain Protocol](#on-chain-protocol)
4. [System Architecture](#system-architecture)
5. [Components](#components)
6. [Quick Start — Honest Mode](#quick-start--honest-mode)
7. [Byzantine Demo — Malicious Node](#byzantine-demo--malicious-node)
8. [Explorer Dashboard](#explorer-dashboard)
9. [Tests](#tests)
10. [Known Limitations](#known-limitations)
11. [Dev Accounts](#dev-accounts)

---

## Motivation

When an LLM inference is outsourced, the requester has no way to verify:
- that the provider ran the model they claimed to run, and not a smaller/cheaper one
- that the activations (intermediate computations) in the reported receipt are genuine

This PoC shows how **Freivalds' algorithm** and **on-chain model commitments** solve both problems at a fraction of the cost of re-running inference, with fraud detection posted as tamper-evident transactions on VeChain.

---

## How CommitLLM Works

### Model Commitment

At startup, every inference node computes a **model commitment**: a SHA-256 fingerprint over samples from five weight tensors (at indices 0, n/4, n/2, 3n/4, n−1 across all named weight parameters). This is deterministic for a given model version and cannot be forged without loading the actual weights.

```python
# commitllm/prover.py  _compute_model_commitment()
indices = sorted({0, n//4, n//2, 3*n//4, n-1})
for idx in indices:
    sample = named_params[idx].flatten()[:256]   # 256 float32 values
    h.update(sample.tobytes())
commitment = h.hexdigest()   # 64-char hex string
```

### Layer Hook

During generation, a forward hook is registered on the **middle `o_proj` layer** (attention output projection). This layer is square `[hidden_dim × hidden_dim]`, has no post-projection normalisation in Qwen2.5, and produces a compact receipt (~28 KB per inference — within VeChain's 64 KB transaction limit).

The hook captures two vectors at the last token position:
- `x` — the input to the layer (hidden_dim floats)
- `z` — the output of the layer (hidden_dim floats)

These are recorded in a **`LayerTrace`** and serialised into the on-chain receipt.

### Freivalds Verification

The checker verifies `z ≈ W @ x` using **Freivalds' algorithm** without recomputing the full matrix multiplication:

```
For 20 independent trials:
  1. Sample a random binary vector r ∈ {0,1}^hidden_dim
  2. Compute  lhs = r · z                 (dot product, O(n))
  3. Compute  rhs = (r · W) · x           (two O(n) dot products)
  4. Reject if |lhs − rhs| / max(|lhs|, |rhs|, 1) > 0.05
```

A cheating prover is caught with probability ≥ 1 − (1/2)²⁰ ≈ 99.9999%.  
All arithmetic uses float64 to tolerate the fp16 accumulation noise from the model.

### Receipt Structure

```json
{
  "model_name":       "Qwen/Qwen2.5-1.5B-Instruct",
  "model_commitment": "<64-char sha256>",
  "input_hash":       "<sha256 of prompt>",
  "output_hash":      "<sha256 of output text>",
  "layer_traces": {
    "model.layers.13.self_attn.o_proj": {
      "x": [1536 floats, 6 sig-figs],
      "z": [1536 floats, 6 sig-figs]
    }
  }
}
```

Activations are rounded to 6 significant figures on serialisation (reduces size from ~600 KB to ~28 KB while keeping Freivalds relative error below 0.01%).

---

## On-Chain Protocol

All messages are sent to a fixed well-known address so nodes can filter by recipient:

```
INFERENCE_ADDR = 0x000000000000000000696e666572656e636541646472
```

Each message is encoded as: `<4-byte ASCII magic> + <compact JSON>`

| Magic | Type | Direction | Purpose |
|-------|------|-----------|---------|
| `INFR` | `InferenceRequest` | User → chain | Request inference; contains model name, prompt, max_tokens |
| `INRS` | `InferenceResponse` | Inference node → chain | Response; contains output text + serialised receipt |
| `INCH` | `InferenceChallenge` | Checker → chain | Verdict: references the INRS tx ID, reports VALID or FRAUD + reason |

### Conversation Flow

```
  User / Open WebUI
       │
       │  POST /v1/chat/completions
       ▼
  openai-proxy (:8000)
       │  posts INFR tx  ──────────────────────────────────────────┐
       │  polls for matching INRS by request_id                    │
       ▼                                                           │
  VeChain Thor (:8669)  ◀─────── INFR, INRS, INCH txs ───────────▶│
       │                                                           │
       ├──▶  inference-node  reads INFR, runs model, posts INRS ──┘
       │
       └──▶  checker-node   reads INRS, verifies receipt, posts INCH
                               │
                               ├── VALID  → on-chain proof accepted
                               └── FRAUD  → commitment mismatch / Freivalds fail

  explorer (:8088)  reads all INFR + INRS + INCH, shows live dashboard
```

### Gas Estimation

Transactions are self-sized using VeChain's intrinsic gas rules:

```
gas = 5000 + Σ clauses (16000 + 68×non_zero_bytes + 4×zero_bytes)
```

A typical INRS receipt (~28 KB) costs roughly **2.2M gas**, well within the 40M block gas limit.

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Docker Compose                                                  │
│                                                                  │
│  ┌──────────────┐   ┌──────────────┐   ┌───────────────────┐   │
│  │  open-webui  │   │ openai-proxy │   │   explorer        │   │
│  │   :3000      │──▶│   :8000      │   │   :8088           │   │
│  └──────────────┘   └──────┬───────┘   └────────┬──────────┘   │
│                             │                    │              │
│                    ┌────────▼────────────────────▼──────────┐  │
│                    │         VeChain Thor solo               │  │
│                    │              :8669                      │  │
│                    └────────┬──────────────────┬────────────┘  │
│                             │                  │               │
│                    ┌────────▼────────┐  ┌──────▼───────────┐  │
│                    │  checker-node   │  │ malicious-node*  │  │
│                    │  (verifier)     │  │ (byzantine PoC)  │  │
│                    └─────────────────┘  └──────────────────┘  │
│   * profile: byzantine                                          │
└─────────────────────────────────────────────────────────────────┘

  ┌───────────────────────────────────┐
  │  Local (Apple Silicon MPS)        │
  │                                   │
  │  inference-node  OR  malicious-node│
  │  ./start_local.sh  OR             │
  │  ./start_malicious.sh             │
  └───────────────────────────────────┘
```

The inference node runs locally to leverage Apple Silicon MPS (Docker on Mac cannot access the GPU). Everything else runs in Docker.

---

## Components

### `py/inference_node.py` — Honest Inference Node

Polls for `INFR` messages, runs `CommitLLMProver.generate_with_proof()`, and posts the result as an `INRS` transaction. Only handles requests for the model it was started with.

```
./start_local.sh [private_key] [model] [thor_url]
```

Default key: `7b067f53…` | Default model: `Qwen/Qwen2.5-1.5B-Instruct`

### `py/checker_node.py` — Checker Node

Polls for `INRS` messages, loads the **claimed** model locally, and runs `CommitLLMVerifier.verify()`. Posts an `INCH` transaction with the verdict and reason.

Runs in Docker (`checker` service). Shares the HuggingFace model cache with the host.

### `py/openai_proxy.py` — OpenAI-Compatible Proxy

Translates OpenAI `POST /v1/chat/completions` into the VeChain INFR/INRS flow:
1. Records the current block number
2. Posts an `INFR` transaction
3. Polls new blocks for a matching `INRS` (by `request_id`)
4. Returns the response in OpenAI format (timeout: 300s)

Exposes `GET /v1/models` so Open WebUI can discover available models.

### `py/explorer.py` — Inference Explorer

Live dashboard at `:8088`. Scans all blocks from genesis (skipping empty blocks via `gasUsed` check), decodes every `INFR`/`INRS`/`INCH` message, and presents them as a searchable table.

Each conversation shows:
- Request ID, block, timestamp, source (USER / INTERNAL), model, prompt
- Response text
- CommitLLM proof details: commitment, input/output hashes, layer, dimensions, receipt size
- On-chain verdict: VALID / FRAUD with the checker's reason string

Auto-refreshes every 10 seconds.

### `py/malicious_node.py` — Byzantine Node (PoC)

Described in detail in the [Byzantine Demo](#byzantine-demo--malicious-node) section below.

### `commitllm/` — CommitLLM Python Package

| File | Purpose |
|------|---------|
| `prover.py` | Loads model, registers `o_proj` hook, generates output + receipt |
| `verifier.py` | Loads claimed model, checks commitment, runs Freivalds |
| `receipt.py` | `Receipt` and `LayerTrace` dataclasses; JSON serialisation |

---

## Quick Start — Honest Mode

**Prerequisites:** Docker, Python 3.11+, ~4 GB disk (model cache)

```bash
# 1. Start the Docker stack (thor + checker + proxy + webui + explorer)
cd inference
docker compose up -d

# 2. Start the inference node locally (first run installs deps + downloads model)
./start_local.sh

# 3. Open the chat UI
open http://localhost:3000

# 4. Open the explorer dashboard
open http://localhost:8088
```

The first `./start_local.sh` run creates `.venv` and downloads `Qwen/Qwen2.5-1.5B-Instruct` (~3 GB) from HuggingFace. Subsequent runs start in seconds.

### Running tests

```bash
# Unit tests only (no model download)
docker compose run --rm unit

# Integration tests (downloads both 1.5B and 0.5B models ~4 GB total)
docker compose run --rm integration

# All tests
docker compose run --rm all
```

---

## Byzantine Demo — Malicious Node

This demo shows how the checker catches a dishonest inference provider. The malicious node claims to be running `Qwen/Qwen2.5-1.5B-Instruct` but misbehaves in two alternating ways.

### Fraud Type 1 — Wrong Model (commitment mismatch)

**What the attacker does:** Runs `Qwen/Qwen2.5-0.5B-Instruct` (cheap, fast) but posts an `INRS` claiming to be `1.5B-Instruct`.

**How it's caught:** The receipt carries the **0.5B model commitment**. The checker loads `1.5B-Instruct`, computes its own commitment, and sees a mismatch → FRAUD at Step 1.

```
checker verdict: model commitment mismatch:
  receipt=cb831dce73ff71f3…  expected=a2f4e91b8c3d0527…
```

**Cost to attacker:** 0.5B model uses ~1 GB RAM and runs ~3× faster than 1.5B, but the fraud is detected before Freivalds even runs.

### Fraud Type 2 — Fake Activations (Freivalds failure)

**What the attacker does:** Claims the correct model (`1.5B-Instruct`) and uses the **correct commitment** (learned by observing a prior honest `INRS` on-chain). But the `x` and `z` vectors in the receipt are replaced with random Gaussian noise rather than real model activations.

**How it's caught:** Commitment check passes (correct commitment was copied). But Freivalds computes `r · z` and `r · W · x` for 20 random vectors `r`. With random `x`, `z`, the equation `z ≈ W @ x` fails with overwhelming probability → FRAUD at Step 2.

```
checker verdict: Freivalds failed at layer
  'model.layers.13.self_attn.o_proj':
  trial 0: r·z=14.3821, r·W·x=0.2047, rel_err=1.8734
```

**Key insight:** The attacker never runs any model at all for this fraud type. The output text can be anything — the on-chain math gives it away.

### Alternating Behaviour

The malicious node alternates between the two fraud types on successive requests:

```
Request 1  →  wrong-model      →  FRAUD (commitment mismatch)
Request 2  →  fake-activations →  FRAUD (Freivalds failed)
Request 3  →  wrong-model      →  FRAUD (commitment mismatch)
...
```

Both fraud types use the 0.5B model to generate the **visible output text**, so responses look like real LLM answers. Only the on-chain proof verification reveals the fraud.

### Running the Byzantine Demo

```bash
# Step 1: start the honest stack and send a few messages
#   (this puts honest INRS receipts on-chain that the malicious node will read)
docker compose up -d       # thor + checker + proxy + webui + explorer
./start_local.sh           # honest inference node

# Step 2: stop the honest inference node (Ctrl-C)

# Step 3: start the malicious node
./start_malicious.sh

# --- OR run in Docker (no GPU, CPU-only, 0.5B is fast enough for demo) ---
docker compose --profile byzantine up malicious

# Step 4: send messages from Open WebUI (localhost:3000)
#   Explorer (localhost:8088) shows alternating FRAUD verdicts with distinct reasons
```

At startup, the malicious node scans the chain for any honest `INRS` to extract the 1.5B commitment and layer metadata needed for the `fake-activations` fraud type. If no honest `INRS` exists yet, it runs `wrong-model` until one appears, then switches to alternating.

### What the Explorer Shows

| Request | INRS output | Verdict | Reason |
|---------|-------------|---------|--------|
| "Name the planets" | "Mercury, Venus…" (from 0.5B) | **FRAUD** | commitment mismatch |
| "Tell me a joke" | "Why did the chicken…" (from 0.5B) | **FRAUD** | Freivalds failed at layer… |
| "What is 2+2?" | "2 + 2 = 4" (from 0.5B) | **FRAUD** | commitment mismatch |

The output text looks legitimate in all cases — only the on-chain proof distinguishes honest from fraudulent responses.

---

## Explorer Dashboard

```
http://localhost:8088
```

| Column | Description |
|--------|-------------|
| Block | Block number of the `INFR` transaction |
| Time (UTC) | Block timestamp |
| Source | USER (chat prompt) or INTERNAL (Open WebUI system prompt) |
| Model | Short model name |
| Prompt | First 55 characters |
| Status | PENDING / RESPONDED / VALID / FRAUD |
| Response | First 55 characters of output |

Click any row to expand a detail panel showing full prompt, response, CommitLLM proof fields (commitment, input/output hashes, layer name, dimensions, receipt size), and the on-chain INCH verdict with reason.

Stats bar shows totals: requests / responded / pending / verified / fraud.

The scanner uses a `gasUsed > 0` pre-check to skip empty blocks, making historical full-chain scans ~50× faster than fetching every expanded block.

---

## Tests

```
commitllm/tests/
  test_freivalds.py     unit tests — Freivalds algorithm correctness
  test_receipt.py       unit tests — Receipt serialisation / round-trips
  test_honest.py        integration — honest prover + verifier passes
  test_fraud.py         integration — wrong-model and tampered activations caught
```

Run unit tests (no model download needed):
```bash
docker compose run --rm unit
```

Run all tests (downloads Qwen2.5-1.5B and Qwen2.5-0.5B ~4 GB):
```bash
docker compose run --rm all
```

---

## Known Limitations

### Output substitution (not detected in v1)

The checker verifies the model commitment and the layer activations, but does **not** verify that `sha256(INRS.output) == receipt.output_hash`. This means a malicious node could:
1. Run the correct model on the correct prompt → obtain valid `x`, `z`, and commitment
2. Post an `INRS` with a different `output` text but the genuine receipt

The checker would post VALID, yet the served text was not what the model produced.

**Mitigation path:** Add an output hash check to the `INCH` verification step. This is straightforward once the protocol includes a commitment to the output (e.g., having the checker re-sample generation with a fixed seed, or using a VRF-based output binding scheme).

### Single-layer proof

Only the middle `o_proj` layer is hooked to keep receipt size within VeChain's 64 KB transaction limit. A single layer is sufficient to detect wrong-model and fake-activation fraud, but does not prove that every layer was computed correctly.

**Mitigation path:** Multi-layer receipts, or a recursive Freivalds scheme that compresses multiple layer checks into a single proof.

### Checker loads full model

The checker downloads and loads the full model weights to compute the commitment and Freivalds weight matrix `W`. For a 70B model this requires ~140 GB RAM. The `safetensors` format supports random tensor access, so a future optimisation could load only the five sampled tensors for commitment check and the single hooked layer for Freivalds — reducing memory from ~140 GB to ~100 MB.

---

## Dev Accounts

Private keys used throughout the PoC (VeChain solo network only — never use on mainnet):

| Role | Private key (first 8 hex) | Usage |
|------|--------------------------|-------|
| User / OpenAI proxy | `99f05005…` | Posts `INFR` transactions |
| Honest inference node | `7b067f53…` | Posts honest `INRS` transactions |
| Checker node | `f4a1a170…` | Posts `INCH` verdict transactions |
| Malicious node | `35b5cc14…` | Posts fraudulent `INRS` transactions |
