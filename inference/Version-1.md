# Verifiable Inference on VeChain — Version 1
### Smart Contract Coordination + Freivalds Fraud Proofs (No Blob Storage)

---

## Overview

A decentralised marketplace for verifiable LLM inference. Inference nodes stake tokens and serve requests. Checkers verify receipts using Freivalds' algorithm off-chain and post verdicts on-chain. Fraud is detected cryptographically and punished economically via slashing. No changes to VeChain Thor are required.

---

## Actors

| Actor | Role | Requirement |
|---|---|---|
| **User** | Submits inference requests, receives outputs | Wallet + VTHO |
| **Inference Node** | Stakes, runs LLM, posts receipts | Stake + GPU hardware |
| **Checker Node** | Verifies receipts, posts verdicts | Stake + model weights |
| **Challenger** | Anyone who disputes a verdict | Bond + model weights |

---

## Architecture

```
Application Layer
  Open WebUI / any client

Coordination Layer
  InferenceMarketplace.sol
    ├── Model registry (commitments)
    ├── Staking & slashing
    ├── Request/response/verdict routing
    ├── Challenge windows
    └── Payment escrow

Off-chain Compute (always off-chain)
  ├── Inference Node  — runs LLM, produces receipts
  └── Checker Node    — runs Freivalds, posts verdicts

Transport
  VeChain Thor (unchanged) — consensus, finality, events
```

---

## Privacy Model

Prompts and outputs are **never posted on-chain**. Only cryptographic hashes and proofs touch the ledger.

```
User → [HTTPS, encrypted with inference node pubkey] → Inference Node
                                                              ↓ runs inference
User ← [HTTPS, encrypted with user pubkey]          ← Inference Node

On-chain (submitResponse):
  outputHash = sha256(output)       ← binding commitment, not the text
  x_root, z_root                    ← Merkle roots of activations
  modelCommitment                   ← Merkle root of weight matrix rows
```

The `outputHash` anchors the off-chain delivery to the on-chain proof. If the inference node delivers a different output than what the proof covers, the user can prove the discrepancy on-chain and trigger slashing.

---

## Smart Contract Interface

```solidity
contract InferenceMarketplace {

    // ── Model Registry ─────────────────────────────────────────────── //
    // W_root = MerkleRoot over rows of the hook layer weight matrix.
    // Set once at model registration; used to verify individual rows
    // during bisection disputes.
    function registerModel(string model, bytes32 W_root) external;

    // ── Inference Node Registration ────────────────────────────────── //
    function registerInferenceNode(string[] models) external payable;
    function slash(address node) internal;  // called by dispute resolution

    // ── Request Lifecycle ──────────────────────────────────────────── //
    function submitRequest(
        string  model,
        bytes32 inputHash,      // sha256(prompt) — prompt sent off-chain
        bytes32 userPubkey,     // for encrypted output delivery
        uint256 maxNewTokens
    ) external payable returns (bytes32 requestId);

    function submitResponse(
        bytes32 requestId,
        bytes32 outputHash,         // sha256(output)
        bytes32 modelCommitment,    // must match registered W_root
        bytes32 x_root,             // MerkleRoot(x[0]..x[n])
        bytes32 z_root,             // MerkleRoot(z[0]..z[n])
        bytes   xEncoded,           // binary float32 — in event, not storage
        bytes   zEncoded            // binary float32 — in event, not storage
    ) external;

    // ── Verification ───────────────────────────────────────────────── //
    function submitVerdict(
        bytes32 requestId,
        bool    valid,
        string  reason
    ) external onlyRegisteredChecker;

    // ── Dispute Resolution ─────────────────────────────────────────── //
    // Non-interactive bisection: challenger posts entire fraud proof
    // in one transaction. See "Bisection Protocol" section below.
    function submitFraudProof(
        bytes32   requestId,
        uint256   disputedIndex,    // element i where z[i] ≠ (W·x)[i]
        int256[]  W_row,            // row i of W (calldata, ~3KB–16KB)
        bytes32[] W_merklePath,     // Merkle proof W[i] is in registered model
        bytes32[] x_merklePath,     // Merkle proof x[i]
        bytes32[] z_merklePath,     // Merkle proof z[i]
        int256    x_i,              // claimed x[i]
        int256    z_i               // claimed z[i]
    ) external payable;             // bond required

    // ── Settlement ─────────────────────────────────────────────────── //
    function finalizeResponse(bytes32 requestId) external;
    // callable after challenge window + no successful fraud proof
    // releases payment to inference node, fee to checkers
}
```

---

## CommitLLM Receipt

The receipt is produced off-chain by the inference node and committed on-chain via Merkle roots. It contains everything needed to verify the computation.

```
Receipt {
  model_name:         "Qwen/Qwen2.5-1.5B-Instruct"
  model_commitment:   MerkleRoot(rows of W)          ← must match registry
  input_hash:         sha256(prompt)
  output_hash:        sha256(output)
  layer_name:         "model.layers.12.self_attn.o_proj"
  x[]:                input activations at last token, float32
  z[]:                output activations at last token, float32
}

Invariant:  z = W · x    (where W is the hook layer weight matrix)
```

The hook layer is the middle `o_proj` (attention output projection) — square `[hidden × hidden]`, no post-projection normalisation in Qwen2.5. One layer per receipt keeps the size within VeChain's 64KB transaction limit.

### Model Commitment

```
W_root = MerkleRoot(
  sha256(W[0]),    ← row 0 of weight matrix
  sha256(W[1]),
  ...
  sha256(W[n-1])
)
```

Individual rows can be proven against `W_root` with a `log₂(n)` hash path. Stored once in the model registry (32 bytes). The inference node computes this at startup from locally downloaded weights.

---

## Freivalds Verification (Off-chain)

The checker downloads the claimed model, loads the hook layer weights `W`, and verifies `z = W · x` using Freivalds' randomised algorithm:

```python
for trial in range(20):
    r = deterministic_r(requestId, blockHash, trial)  # {0,1}^n
    lhs = r · z
    rhs = (r @ W) · x
    assert |lhs - rhs| / max(|lhs|, |rhs|, 1) < 0.05
```

### Deterministic r vectors

`r` is derived from on-chain entropy rather than random sampling:

```python
r[trial] = keccak256(requestId || blockHash || trial) mod 2
```

This ensures every honest checker running the same request produces **identical intermediate values**. Two checkers who disagree cannot both be honest. The contract can slash the minority without running any computation itself.

**Catches:** lazy/random liars, griefing attacks  
**Does not catch:** colluding majority of checker stake

---

## Bisection Fraud Proof

When a challenger believes a response is fraudulent, they locate the specific element `i` where `z[i] ≠ (W · x)[i]` and prove it on-chain in a **single transaction**.

### Why bisection is necessary

Verifying all `n` elements on-chain requires all `n` rows of `W`:

```
1.5B:  1536 rows × 3KB  = 4.5MB calldata  — exceeds 64KB tx limit
70B:   8192 rows × 16KB = 128MB calldata  — impossible
```

Bisection reduces this to **one row**.

### Non-interactive protocol

The challenger computes all bisection steps locally and posts the result in one transaction:

```
Step 1 — Challenger finds disputed index i:
  Compute y = W · x locally (has the model)
  Find first i where y[i] ≠ z[i]

Step 2 — Challenger posts fraud proof:
  disputedIndex = i
  W_row         = W[i]  (3KB for 1.5B, 16KB for 70B)
  W_merklePath  = Merkle proof W[i] ∈ registered model  (log₂(n) × 32B)
  x_merklePath  = Merkle proof x[i] from x_root         (log₂(n) × 32B)
  z_merklePath  = Merkle proof z[i] from z_root         (log₂(n) × 32B)

Step 3 — Contract adjudicates (~15k–50k gas):
  1. Verify W_merklePath against W_root in registry
  2. Verify x_merklePath against x_root in response
  3. Verify z_merklePath against z_root in response
  4. Compute: y_i = sum(W[i][j] * x[j])  — one dot product
  5. Compare y_i with z[i]:
       y_i ≠ z[i]  →  inference node SLASHED, challenger rewarded
       y_i == z[i] →  challenger SLASHED (false accusation)
```

### Why one honest challenger is sufficient

The colluding checker problem (majority votes VALID on fraud) is broken by the universal challenger model. Anyone with the model can post a fraud proof. Collusion requires corrupting every potential challenger globally, not just a registered set. The attack surface is the whole network.

---

## Optimistic Verification Flow

```
Block N:    submitResponse() posted
            challenge window opens (e.g. 240 blocks ≈ 40 min)

Blocks N–N+240:
            Checkers verify off-chain
            Honest checkers post VALID verdicts
            Fraudulent responses: challenger posts fraud proof → instant slash

Block N+240:
            No successful fraud proof?
            finalizeResponse() called
            Inference node receives payment
            Checkers receive verification fee
```

In the happy path (no fraud), the only on-chain cost is three transactions per inference (request, response, verdict) plus the finalization call.

---

## Slashing & Economics

```
Slashing triggers:
  1. submitFraudProof() succeeds  →  inference node slashed
  2. False fraud proof             →  challenger slashed
  3. Checker minority verdict      →  minority checkers slashed

Slash distribution:
  Inference node stake  →  split among fraud-catching challengers
                           + refund to user

Incentive alignment:
  Inference nodes:   honest behaviour ≤ stake value
  Checkers:          earn fee per valid verdict; lose stake if lying
  Challengers:       profit from catching fraud; lose bond if wrong
```

---

## Throughput Analysis

What goes on-chain per inference:

| Transaction | Calldata | Gas | Notes |
|---|---|---|---|
| `submitRequest` | ~128B | ~50k | hashes only |
| `submitResponse` | 64B + x/z in events | ~200–500k | binary float32 |
| `submitVerdict` | ~64B | ~30k | per checker |
| `finalizeResponse` | ~32B | ~30k | after window |
| `submitFraudProof` | ~20KB | ~200k | rare |

x/z are emitted as **events** (not stored in contract state). Event data costs ~8 gas/byte and is readable by all nodes but not persistent in contract storage.

```
1.5B model receipt (x + z = 12KB):
  Event gas:     12KB × 8 = 96k gas
  Total/response: ~150k gas
  Throughput:    40M / 150k ≈ 260/s

70B model receipt (x + z = 64KB):
  Event gas:     64KB × 8 = 512k gas
  Total/response: ~560k gas
  Throughput:    40M / 560k ≈ 70/s
```

VeChain block time: 10 seconds. Block gas limit: 40M (expandable to 500M via VIP-222).

---

## Scalability at 70B

| Component | 1.5B | 70B | Concern |
|---|---|---|---|
| On-chain dot product (dispute) | ~15k gas | ~50k gas | No |
| One row in calldata | 3KB | 16KB | No — fits in 64KB |
| Merkle proof per element | 11 × 32B | 13 × 32B | No |
| x/z in events | 12KB | 64KB — at tx size limit | Needs binary encoding |
| Model Merkle tree computation | Fast (startup) | ~30s (startup) | One-time |
| Challenger downloads to verify | ~3MB | ~16MB | Manageable |

The protocol scales to 70B. The binding constraint is the 64KB transaction limit for receipts, addressable by binary encoding (float32 instead of JSON). See Version 2 for a cleaner solution.

---

## Trust Model

| Claim | Verified by | Trust required |
|---|---|---|
| Model commitment is correct | Anyone who downloads the model | Trust inference node at registration |
| `z = W · x` | Freivalds (off-chain) | Deterministic, reproducible |
| Checker ran Freivalds honestly | Majority stake + challenger | Trust majority of checker stake |
| Fraud proof correct | Contract (on-chain dot product) | **Trustless** |
| Output matches receipt | `outputHash` anchors delivery | Trust inference node delivered matching output |

The remaining trust gaps — model registration and checker majority — are addressable by ZK proofs of weight loading and ZK Freivalds respectively. The tooling (EZKL, Risc0) is not yet practical for 1B+ models but is the correct long-term direction.

---

## Protocol Limitations (addressed in Version 2)

1. **Receipt size**: x/z as events approach the 64KB tx limit for 70B models
2. **Merkle roots for x/z**: requires a separate commitment scheme maintained by the inference node
3. **Throughput ceiling**: x/z event gas competes with block gas limit
4. **Dispute element proofs**: Merkle proofs for individual x/z elements require a Merkle tree over activations
