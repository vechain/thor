# eth-tx verification toolkit

Manual verification tooling for the eth-tx branch (specs 1 + 2 + 3). See `docs/superpowers/specs/2026-04-24-eth-tx-verification-toolkit-design.md` and `docs/superpowers/plans/2026-04-24-eth-tx-verification-toolkit.md`.

## Components

- **`txblast/`** — Go batch tx blaster. Sends 5 tx variants × 2 submit paths every 2 s against a solo node; asserts invariant V1 (REST + RPC submit paths are symmetric).
  ```
  go run ./tools/eth-tx-verify/txblast
  ```

- **`dapp/`** — single-file HTML DApp. Guides a MetaMask user through add-chain → connect → send 0x02 → deploy Counter → verify CREATE address derivation (spec 3 D1 end-to-end).
  ```
  cd tools/eth-tx-verify/dapp && npx -y serve -l 8080
  ```

- **Server-side:** `api/rpc/middleware_log.go` emits one structured `eth-rpc` slog line per inbound /rpc request (V2 / V2' invariants). Enabled by `--enable-api-logs` on thor.

## Quick-start smoke

```bash
# Terminal 1 — solo
./bin/thor solo --on-demand --api-eth-rpc-enabled --enable-api-logs

# Terminal 2 — txblast (60 seconds ≈ 30 batches)
go run ./tools/eth-tx-verify/txblast

# Terminal 3 — dapp (browser at http://localhost:8080)
cd tools/eth-tx-verify/dapp && npx -y serve -l 8080
```

## Acceptance
See `docs/superpowers/specs/2026-04-24-eth-tx-verification-toolkit-design.md` §6.2 for the full A1–A5 acceptance gates.
