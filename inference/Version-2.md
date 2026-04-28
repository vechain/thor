# Verifiable Inference on VeChain — Version 2
### Smart Contract Coordination + Freivalds Fraud Proofs + Blob Storage

---

## Overview

Version 2 extends the Version 1 architecture with blob-carrying transactions. Receipts (activation vectors x/z) are posted as blobs rather than calldata or events. This decouples receipt data from block execution gas, dramatically increases throughput, and replaces the Merkle commitment scheme for x/z with KZG polynomial commitments — the same primitive used natively by the blob transaction format.

**Requires:** A VeChain VIP adding blob-carrying transactions, KZG commitments, and a point evaluation precompile.

---

## What Changes from Version 1

| | Version 1 | Version 2 |
|---|---|---|
| Receipt storage | Events (calldata) | Blobs |
| x/z commitment | Merkle root (manual) | KZG commitment (automatic) |
| Element proof in dispute | Merkle path (log₂(n) × 32B) | KZG opening proof (48B flat) |
| Receipt gas cost | ~8 gas/byte (event) | ~1–3 gas/byte (blob) |
| Receipt gas pool | Shared with execution | Separate blob gas market |
| 70B at tx size limit | Yes (64KB exactly) | No — blobs support 128KB+ |
| Throughput (70B) | ~70/s | ~800/s |

Everything else — staking, slashing, bisection logic, privacy model, Freivalds, deterministic r — is identical to Version 1.

---

## Blob Storage Properties

```
Blob data:
  ├── NOT stored in EVM state — contracts cannot read blob contents directly
  ├── Guaranteed available for retention period (e.g. 7–18 days)
  ├── Pruned after retention period from full nodes
  ├── Cheap: ~1–3 gas/byte vs ~16 gas/byte for calldata
  └── Separate gas market — does not compete with block execution gas

KZG commitment (automatic):
  ├── Every blob posting generates a 48-byte KZG commitment C
  ├── C commits to every element of the blob as a polynomial evaluation
  └── Point evaluation precompile: verify element i = v against C in ~50k gas
```

The retention period is designed to exceed the challenge window. If the challenge window is 240 blocks (~40 minutes), blob data is available for days — more than sufficient.

---

## Architecture

```
Application Layer
  Open WebUI / any client

Coordination Layer
  InferenceMarketplace.sol
    ├── Model registry (W_root as Merkle root of weight rows)
    ├── Staking & slashing
    ├── Request/response/verdict routing
    ├── Challenge windows
    ├── Payment escrow
    └── Point evaluation precompile calls (for dispute resolution)

Off-chain Compute (always off-chain)
  ├── Inference Node  — runs LLM, produces receipts, posts blobs
  └── Checker Node    — reads blobs, runs Freivalds, posts verdicts

Transport
  VeChain Thor + blob transactions — consensus, finality, blobs
```

---

## Privacy Model

Identical to Version 1. Prompts and outputs are never posted on-chain.

```
User → [HTTPS, encrypted with inference node pubkey] → Inference Node
                                                              ↓ runs inference
User ← [HTTPS, encrypted with user pubkey]          ← Inference Node

On-chain (submitResponse):
  outputHash = sha256(output)     ← permanent in contract state
  C_x        = KZG(x blob)        ← 48-byte commitment, permanent
  C_z        = KZG(z blob)        ← 48-byte commitment, permanent

Blob (temporary, pruned after retention period):
  blob 1: x[] binary float32      ← accessible to checkers during window
  blob 2: z[] binary float32      ← accessible to checkers during window
```

---

## Smart Contract Interface

Changes from Version 1 are highlighted with `← V2`.

```solidity
contract InferenceMarketplace {

    // ── Model Registry ─────────────────────────────────────────────── //
    // W_root = MerkleRoot over rows of the hook layer weight matrix.
    // Unchanged from V1 — model weights are not posted as blobs since
    // they are permanent reference data, not temporary receipt data.
    function registerModel(string model, bytes32 W_root) external;

    // ── Request Lifecycle ──────────────────────────────────────────── //
    function submitRequest(
        string  model,
        bytes32 inputHash,
        bytes32 userPubkey,
        uint256 maxNewTokens
    ) external payable returns (bytes32 requestId);

    function submitResponse(
        bytes32 requestId,
        bytes32 outputHash,
        bytes32 modelCommitment,
        bytes48 C_x,            // ← V2: KZG commitment to x blob
        bytes48 C_z             // ← V2: KZG commitment to z blob
        // x[] and z[] carried as blobs in the same transaction, not as args
    ) external;

    // ── Verification ───────────────────────────────────────────────── //
    function submitVerdict(
        bytes32 requestId,
        bool    valid,
        string  reason
    ) external onlyRegisteredChecker;

    // ── Dispute Resolution ─────────────────────────────────────────── //
    function submitFraudProof(
        bytes32   requestId,
        uint256   disputedIndex,    // element i where z[i] ≠ (W·x)[i]
        int256[]  W_row,            // row i of W (calldata)
        bytes32[] W_merklePath,     // Merkle proof W[i] ∈ registered model
        bytes48   x_kzgProof,       // ← V2: KZG opening proof for x[i]
        bytes48   z_kzgProof,       // ← V2: KZG opening proof for z[i]
        int256    x_i,
        int256    z_i
    ) external payable;

    // ── Settlement ─────────────────────────────────────────────────── //
    function finalizeResponse(bytes32 requestId) external;
}
```

---

## KZG Commitments Replace Merkle Roots for x/z

In Version 1, the inference node must manually compute Merkle trees over x and z and post the roots. In Version 2, this is automatic.

**Version 1 (Merkle):**
```python
x_root = MerkleRoot(x[0], x[1], ..., x[n-1])  # manual computation
z_root = MerkleRoot(z[0], z[1], ..., z[n-1])  # manual computation
# posted as part of submitResponse() calldata
```

**Version 2 (KZG, automatic):**
```python
# Inference node posts blobs containing x and z
# Chain automatically computes KZG commitments C_x and C_z
# Inference node reads C_x, C_z from transaction receipt
# Posts C_x and C_z in submitResponse() calldata
```

The commitment is computed by the chain's blob processing, not by the inference node. No additional cryptographic tooling required.

---

## Bisection Fraud Proof with KZG

The bisection logic is identical to Version 1 — find the disputed index `i`, prove it in one transaction. The only change is how individual elements are proven.

**Version 1 (Merkle path):**
```
Prove x[i] against x_root:
  path = [sibling_0, sibling_1, ..., sibling_10]   ← 11 × 32B = 352B
  Contract: recompute root from path, compare with stored x_root
```

**Version 2 (KZG opening proof):**
```
Prove x[i] against C_x:
  proof = kzg_open(x_blob, i)                       ← 48B flat
  Contract: pointEval(C_x, i, x[i], proof)          ← precompile call ~50k gas
```

The proof is smaller (48B vs 352B+), simpler (no tree structure), and uses the chain's native precompile.

### Full dispute transaction (Version 2):

```
submitFraudProof():
  W_row[]:      16KB  (row i of W, same as V1)
  W_merklePath: 416B  (13 × 32B, same as V1)
  x_kzgProof:   48B   ← V2: replaces x Merkle path
  z_kzgProof:   48B   ← V2: replaces z Merkle path
  ─────────────────
  Total:        ~17KB  (vs ~20KB in V1)
  Gas:          ~200k  (similar, precompile is cheap)
```

---

## Throughput Analysis

### Per-inference gas breakdown

```
submitRequest:      ~50k gas   (unchanged)
submitResponse:     ~80k gas   ← V2: only hashes + 2 KZG commitments (96B)
                    + blob fee (separate gas pool, does not count against 40M)
submitVerdict:      ~30k gas   (unchanged)
finalizeResponse:   ~30k gas   (unchanged)
submitFraudProof:   ~200k gas  (rare, similar to V1)
```

### Throughput (execution gas limited, not data limited)

```
V1 (70B, events):
  Execution + event gas per response:  ~560k
  Throughput: 40M / 560k ≈ 70/s

V2 (70B, blobs):
  Execution gas per response:  ~80k   (blobs in separate pool)
  Throughput: 40M / 80k ≈ 500/s
```

### Blob capacity (separate pool)

```
Blob capacity per block: e.g. 6 blobs × 128KB = 768KB
70B receipt (x + z):  64KB = 0.5 blobs
Blob-limited throughput: 6 / 0.5 = 12 responses per block × 6 blocks/min = 72/min
```

In practice the execution gas limit is hit first at high throughput, so blob capacity is not the binding constraint until ~500 requests/second.

### Comparative throughput

| Model | V1 (events) | V2 (blobs) |
|---|---|---|
| 1.5B | ~260/s | ~500/s |
| 7B | ~130/s | ~500/s |
| 70B | ~70/s | ~500/s |

With VIP-222 raising the block gas limit to 500M: both versions scale proportionally.

---

## Blob Retention and Challenge Windows

The challenge window must be shorter than the blob retention period.

```
Blob retention:    e.g. 7 days
Challenge window:  e.g. 240 blocks = 40 minutes

Margin:            massive — blobs available 10,000× longer than needed
```

After the challenge window closes and the response is finalised, the blob data can be pruned. No permanent storage burden. Checkers only need to read blobs during the active window.

For archival purposes (e.g. the Inference Explorer scanning historical blocks), archive nodes retain blobs indefinitely. This is the same model as Ethereum — full nodes prune, archive nodes keep.

---

## VeChain VIP Requirements

Version 2 requires a VeChain Improvement Proposal adding:

### 1. Blob-carrying transactions
Transactions can carry one or more blobs of up to 128KB each. Blobs are broadcast with the transaction but not stored in EVM state.

### 2. KZG commitment scheme
For each blob posted, the chain computes and records a 48-byte KZG polynomial commitment. The commitment is accessible to contracts via the transaction receipt.

### 3. Point evaluation precompile
A precompile (e.g. at a fixed address) implementing:

```
pointEval(
    bytes48 commitment,   // KZG commitment C
    uint256 position,     // index i
    int256  value,        // claimed value v
    bytes48 proof         // opening proof π
) → bool
```

Verifies that the blob committed to by `C` evaluates to `v` at position `i`. Gas cost: ~50k. Implementation: ~200 lines of Go (BLS12-381 pairing), directly portable from the Ethereum EIP-4844 implementation.

### 4. Separate blob gas market
Blob gas priced and limited independently of execution gas. Blob gas does not compete with contract execution for block space. Full nodes may prune blob data after the configured retention period.

### Reference implementation
EIP-4844 (Ethereum): https://eips.ethereum.org/EIPS/eip-4844  
The KZG precompile and blob transaction format are directly reusable.

---

## Scalability at 70B

| Component | V1 | V2 | Change |
|---|---|---|---|
| Receipt on-chain cost | ~512k gas (events) | ~0 execution gas (blobs) | Decoupled |
| x/z commitment | Merkle root (manual) | KZG commitment (automatic) | Simpler |
| Dispute element proof | Merkle path (352B) | KZG opening (48B) | Smaller |
| 70B at tx size limit | Yes | No — blobs separate | Resolved |
| Throughput (70B) | ~70/s | ~500/s | 7× |
| Receipt data permanence | Forever (events in blocks) | Pruned after retention | Cleaner |

---

## Trust Model

Identical to Version 1, with one improvement:

| Claim | Verified by | V1 Trust | V2 Trust |
|---|---|---|---|
| Model commitment correct | Download + recompute | Trust node at registration | Same |
| `z = W · x` | Freivalds off-chain | Deterministic, reproducible | Same |
| Checker honest | Majority stake + challenger | Trust majority | Same |
| Fraud proof correct | Contract dot product | **Trustless** | **Trustless** |
| x[i] matches blob | Merkle path (V1) / KZG proof (V2) | Manual scheme | **Chain-native** |
| z[i] matches blob | Merkle path (V1) / KZG proof (V2) | Manual scheme | **Chain-native** |

KZG commitments are computed by the chain, not the inference node. The inference node cannot post a wrong commitment for a blob — the commitment is derived deterministically by the chain's blob processing.

---

## Long-term Direction: ZK Freivalds

Both Version 1 and Version 2 leave one gap: the checker's Freivalds execution is not verifiable on-chain without re-running it. The long-term solution is a ZK proof of Freivalds:

```
Checker generates ZK proof:
  "I ran Freivalds on (x, z, W) where KZG(W) = C_W, and the result is VALID/FRAUD"

Contract:
  verifyZKProof(proof, C_W, C_x, C_z, result)  →  bool
  ~300k gas, trustless
```

In Version 2, the KZG commitments to x and z are already on-chain as part of blob handling. A ZK circuit over KZG-committed inputs is the natural next step. Tooling (EZKL, Risc0) is advancing rapidly but not yet practical for 1B+ models. Version 2's commitment infrastructure is forward-compatible with this upgrade — no protocol change needed when ZK Freivalds becomes practical.

---

## Migration from Version 1

Version 2 is additive. The contract interface changes minimally:
- `submitResponse()`: replace `x_root, z_root` (bytes32) with `C_x, C_z` (bytes48)
- `submitFraudProof()`: replace Merkle paths with KZG proofs

Off-chain nodes:
- Inference node: post blobs alongside `submitResponse()`, read KZG commitments from receipt
- Checker node: read blobs from block data instead of event logs
- Challenger: use `kzg_open()` instead of Merkle tree library

The staking, slashing, bisection logic, Freivalds implementation, and privacy model are unchanged.
