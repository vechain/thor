# eth-tx Verification Toolkit

Hands-on verification tooling for the `eth-tx` branch (specs 1 + 2 + 3 stacked on top of `evm-upgrades`). Three components that together exercise every code path touched by the branch:

| Component | What it verifies | Location |
|---|---|---|
| `txblast` (Go binary) | V1 — REST and RPC submit paths symmetrically accept all 5 tx variants | `tools/eth-tx-verify/txblast/` |
| `dapp` (HTML + MetaMask) | spec 3 D1 — 0x02 CREATE uses eth-style address derivation, end-to-end through a real wallet | `tools/eth-tx-verify/dapp/` |
| `middleware_log.go` (thor) | V2 / V2' — every `/rpc` request produces exactly one structured log line with `method` field | `api/rpc/middleware_log.go` |

Design: `docs/superpowers/specs/2026-04-24-eth-tx-verification-toolkit-design.md`
Plan: `docs/superpowers/plans/2026-04-24-eth-tx-verification-toolkit.md`

---

## Pre-flight

```bash
# 1. You are on the toolkit branch (or later) — specs 1/2/3 must be implemented.
git rev-parse --abbrev-ref HEAD            # expect: eth-tx-verify-toolkit

# 2. Build thor once. Includes the request-logging middleware from Phase 1.
go build -o bin/thor ./cmd/thor

# 3. Confirm Phase 1 unit tests are green (this is acceptance A1).
go test ./api/rpc/... -count=1             # expect: PASS

# 4. Tools available
node --version                             # any modern v18+ for npx serve
# MetaMask installed in your browser (Chrome / Firefox / Brave) for the DApp step.
```

---

## Running

Three terminals. Start them in this order.

### Terminal 1 — thor solo with RPC + request logger

```bash
./bin/thor solo --on-demand --api-cors='*' --api-eth-rpc-log-file ./eth-rpc.log --verbosity 3
```

Flags explained:
- `--on-demand` — mines a block immediately after each tx lands. Keeps receipt queries fast.
- `--api-cors='*'` — let MetaMask and the DApp page reach `/rpc`. solo defaults to empty (cross-origin blocked at preflight). Lock down to specific origins for non-dev use.
- `--api-eth-rpc-log-file <path>` — single switch: mounts `POST /rpc` (the spec-2 Ethereum JSON-RPC namespace) **and** appends one structured JSON log line per request to `<path>` (`O_APPEND|O_CREATE`). Empty/absent = `/rpc` is not mounted at all. The eth-RPC logger is deliberately a separate file so REST chatter doesn't drown verification output.
- `--verbosity 3` — INFO level; affects thor's main stdout, unrelated to the dedicated eth-rpc file.

In a second pane, tail the file to watch traffic:

```bash
tail -f ./eth-rpc.log | jq -c '{m:.method, ref:.referer, o:.origin, ms:.latency_ms, code:.code}'
```

Example line on a successful `eth_sendRawTransaction` from MetaMask:

```json
{"t":"...","lvl":"info","msg":"eth-rpc","method":"eth_sendRawTransaction","code":0,"latency_ms":4.2,"referer":"","origin":"chrome-extension://nkbihfbeogaeaoehlefnkodbefgpgknn","params_preview":"[\"0x02f8...\"]","result_preview":"0x55a44...b565"}
```

Two fields together attribute every request:

| client | `referer` | `origin` |
|---|---|---|
| MetaMask (Chrome) | `""` (extension strips it) | `chrome-extension://nkbihfbeogaeaoehlefnkodbefgpgknn` |
| MetaMask (Firefox) | `""` | `moz-extension://<uuid>` |
| DApp page JS (bypassing MM) | `http://localhost:8080/` | `http://localhost:8080` |
| txblast | `txblast/eth-tx-verify` | `""` (not a browser) |
| curl / generic tooling | `""` | `""` |

Use `origin` to identify MetaMask — `referer` alone reads as empty for almost all wallet traffic because browser extensions suppress it. CORS forces the browser to set `Origin` on every cross-origin POST, including from extensions, so it's reliable.

```bash
# Just MetaMask traffic
jq -c 'select(.origin | startswith("chrome-extension://") or startswith("moz-extension://"))' eth-rpc.log

# Per-client method totals
jq -r '[
  (if   .origin == ""                                 then (.referer // "non-browser")
   elif .origin | startswith("chrome-extension://")   then "metamask-chrome"
   elif .origin | startswith("moz-extension://")      then "metamask-firefox"
   else "page:" + .origin end),
  .method
] | @tsv' eth-rpc.log | sort | uniq -c | sort -rn
```

### Terminal 2 — txblast

```bash
go run ./tools/eth-tx-verify/txblast
```

Default: 2 s interval, 1 copy of the 10-row matrix per batch. What you want to see — **every row green, V1 HOLDS**:

```
=== Batch 1 @ 2026-04-27T17:05:22Z ===
type    clauses   path   from           txid                                                           submit       receipt
0x00    1         REST   0xa3a4...3ad   0xabc1...def0                                                  OK           MISS
0x00    1         RPC    0xc7c0...001   0x1234...5678                                                  OK           MISS
...
0x02    1         RPC    0x99f0...fd36  0x55a4...b565                                                  OK           MISS
=== summary: submit 10/10  receipt 0/10  errors 0  invariant V1: HOLDS ===
```

The `from` column shows which dev account signed each row — randomly picked from `dev[3..9]` per tx (see "Random signer pool" below). First batch's receipts will be `MISS` (nothing mined yet). From batch 2 onward you'll see `OK(blk=N)` for most rows:

```
=== Batch 2 @ 2026-04-27T17:05:24Z ===
0x00    1         REST   0xa3a4...3ad   0xaaaa...aaaa                                                  OK           OK(blk=8)
0x00    3         RPC    0xc7c0...001   0xbbbb...bbbb                                                  OK           ERR(tx_not_representable)   ← expected
...
=== summary: submit 10/10  receipt 7/10  errors 3  invariant V1: HOLDS ===
```

**Don't panic at `receipt 7/10` / `ERR(tx_not_representable)`.** These are not V1 failures. They are spec-2 **by-design** behavior: `eth_getTransactionReceipt` refuses to project multi-clause VeChain txs to the eth shape (spec 2 §2.3 "non-representable"). Submit still shows 10/10 on both paths. V1 only tracks submit symmetry.

**What matters**:
- `submit 10/10` every batch — both REST and RPC accepted every variant.
- `V1: HOLDS` every batch — the two submit paths are symmetric per (type, clauses) cell.

Stop any time with **Ctrl+C**. txblast catches SIGINT, queries the last batch's receipts, prints a final summary, exits cleanly.

Useful flags:
```bash
go run ./tools/eth-tx-verify/txblast -interval 5s                                    # slower batches
go run ./tools/eth-tx-verify/txblast -batch 3                                        # 30 txs/batch
go run ./tools/eth-tx-verify/txblast -dry-run                                        # build + sign, don't submit
go run ./tools/eth-tx-verify/txblast -url http://localhost:8669                      # override URL
go run ./tools/eth-tx-verify/txblast -receipt-timeout 30s                            # default 15s; raise if solo's block interval is longer
```

### Timing model

Each submitted batch enters a **pending queue**. On every tick, txblast retries `eth_getTransactionReceipt` on every unresolved row across **every** pending batch, and prints a batch only when (a) all its rows resolved, or (b) the batch is older than `-receipt-timeout` (then remaining rows stay `MISS`). This means:

- **On-demand solo** (`--on-demand`): batches print after ~1 tick, receipts usually already `OK(blk=N)`.
- **Timer-based solo** (default 10s block interval): batches may spend multiple ticks in the queue before printing. At 2s interval with 15s timeout, a batch lives for up to ~8 ticks before being printed. This is expected — your output lags ~10-15 seconds behind submission.

### Random signer pool (dev[3..9])

Every tx in a batch is signed by an account picked uniformly at random from the **dev[3..9]** range — seven solo dev accounts with effectively infinite VET / VTHO. dev[0] and dev[1] are deliberately excluded so they remain free for the DApp's MetaMask import and other tooling.

Each pool account has its own independent EthNonce counter (only 0x02 needs sequential nonces; 0x00 / 0x51 use random nonces and ignore the counter). Within a batch the **REST 0x02** and **RPC 0x02** rows are routed to *different* signers chosen up front — even though `eth_sendRawTransaction` now uses `pool.AddLocal` (which queues future-nonce instead of rejecting it), distinct signers keep both rows immediately executable so receipts resolve in the same batch instead of waiting for the queued one to promote.

If a 0x02 ever surfaces `nonce_too_low` / `nonce_too_high` (e.g. another tool sent from the same account behind txblast's back), the next finalized batch resyncs **just that account's** counter from on-chain state — you'll see `[info] nonce resynced  0x… → N`. No three-batch streak required, and other accounts keep their local counters intact.

The `from` column in each batch's output shows which dev account signed each row. The startup banner lists the full pool with current nonces.

### Sending Referer from txblast

txblast tags every outbound HTTP request (RPC + REST + GET) with `Referer: txblast/eth-tx-verify` so server-side `eth-rpc.log` can distinguish its traffic from MetaMask / DApp without payload parsing. See the client-identification matrix above.

### Terminal 3 — DApp

```bash
cd tools/eth-tx-verify/dapp
npx -y serve -l 8080
```

Then browser → `http://localhost:8080` → MetaMask installed → work through the 6-step checklist in `dapp/README.md`.

**Focus on step 4** (Deploy Counter). It's the end-to-end verification of **spec 3 D1** — the eth-style CREATE address derivation. If you see `✅ Addresses match`, spec 3 is validated through a real wallet signing path.

---

## What each invariant looks like when "holding"

| Invariant | Spec § | How to see it | Where to look |
|---|---|---|---|
| **V1** — REST+RPC submit path symmetry | §2.2, §3 | `submit 10/10` and `V1: HOLDS` on every batch | Terminal 2 summary line |
| **V2** — one log line per `/rpc` request | §2.2, §4 | One JSON line per click/tx in `eth-rpc.log` | `tail -f eth-rpc.log` |
| **V2'** — `method` field always present | §2.2 | Every line has `"method":"…"`. Parse/garbage requests show `"method":"(unknown)"`. | `jq -r .method eth-rpc.log \| sort -u` |
| **Spec 3 D1** — 0x02 CREATE = keccak(rlp(from,nonce))[12:] | spec 3 §4 | Green `✅ Addresses match` in DApp panel ④ | DApp browser |

---

## Troubleshooting

### `txblast: chain tag mismatch` or `bad tx`
Thor's solo mode had a bug where `OnDemandTxPool.AddLocal` unconditionally checked `ChainTag()` even for 0x02 txs (which return 0 there and use `ChainID` instead). Already fixed in commit `cdb8f8fc` on this branch. If you rebased onto a version without that fix, 0x02 submits will fail. Apply the same fix that mirrors `txpool.TxPool.Add:709-714`.

### `receipt MISS` persistent for all rows
`--on-demand` must be passed to solo — otherwise blocks only produce on a timer and the receipt-on-next-tick pattern starves. Confirm Terminal 1 shows the `--on-demand` flag on the command line.

### `nonce_too_low` / `nonce_too_high` repeating for one signer
Every pool account has its own EthNonce; on a 0x02 submit error the next finalized batch resyncs *that* account from on-chain state and other accounts keep their local counters. If the same account keeps failing, solo was likely restarted under txblast (its state nonce reset to 0). Restart txblast — startup re-queries every account.

### No `eth-rpc` lines in the log file
Two causes:
1. `--api-eth-rpc-log-file <path>` missing → namespace not mounted, so no logger exists either.
2. The path can't be opened (perm/dir missing) → thor exits at startup with `open eth-rpc log file [...]` — check stderr.

### DApp: MetaMask rejects "Add network"
MetaMask requires `chainId` sent to `wallet_addEthereumChain` to match the RPC's `eth_chainId` exactly. The DApp fetches it live before adding, so a mismatch means your solo instance isn't the one serving `:8669`. Double-check the RPC URL in MetaMask settings.

### DApp: `❌ Addresses match` failed
This is the red signal that spec 3 D1 is broken. Possible causes:
1. Solo is running a stale binary without spec 3 — rebuild thor (`go build -o bin/thor ./cmd/thor`).
2. The tx went through a non-0x02 path somewhere — check Terminal 1 for `method=eth_sendRawTransaction` and confirm `tx.type == 0x02` in the raw.

### `go run ./tools/eth-tx-verify/txblast` fails with "undefined: BuildMatrix"
IDE-level stale cache issue in package-main sibling files. Build authoritatively: `go build ./tools/eth-tx-verify/txblast/...`. The actual compiler is happy.

---

## Acceptance gates

From spec §6.2, listed with what automates vs what's manual:

- **A1** — `go test ./api/rpc/...` all green → fully automated, run on every commit.
- **A2** — 5 consecutive txblast batches, each `submit 10/10` + `V1 HOLDS` → run Terminal 2 for ~12 s.
- **A3** — `eth-rpc.log` shows ≥10 lines per txblast batch (5 RPC submits + 5 receipt queries), each carrying a `"method":"…"` field → `jq -r 'select(.referer=="txblast/eth-tx-verify") | .method' eth-rpc.log | wc -l`.
- **A4** — DApp checklist 1–6 all pass, especially step 4 showing `✅` → manual, needs MetaMask.
- **A5** — Cross-reference any 5 DApp sidebar entries with `eth-rpc.log` rows whose `origin` starts with `chrome-extension://` (or `moz-extension://`) → manual sampling.

Running all five takes ~10 minutes end to end if MetaMask is already installed.

---

## Cleaning up

Stop in this order:
1. Ctrl+C txblast (Terminal 2) — prints final summary.
2. Ctrl+C `npx serve` (Terminal 3) — immediate.
3. Ctrl+C solo (Terminal 1) — flushes logs.

No persistent state; solo's on-disk DB lives in `$HOME/.org.vechain.thor/` and can be deleted safely between runs if you want a clean genesis.
