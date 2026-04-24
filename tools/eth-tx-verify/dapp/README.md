# eth-tx-verify DApp

Verifies spec 1/2/3 of the ETH-Tx branch through a real MetaMask wallet: add chain → connect → send 0x02 VET → deploy Counter with address assertion → read/write contract state.

## Prerequisites

- thor solo running with the eth RPC namespace enabled:
  ```
  ./bin/thor solo --on-demand --api-eth-rpc-enabled --enable-api-logs
  ```
- MetaMask installed in your browser.
- A static file server on port 8080:
  ```
  cd tools/eth-tx-verify/dapp && npx -y serve -l 8080
  ```

Open http://localhost:8080 and work through the panels.

## Verification checklist

For each step, confirm the outcome before proceeding.

1. [ ] "Add VeChain Solo to MetaMask" succeeds: chainId appears in MetaMask's network list (value e.g. `0xe558`).
2. [ ] "Connect Wallet" shows the dev account address, VET balance, and VTHO balance (VTHO via the Energy precompile `0x0...456E65726779`).
3. [ ] "Send 0.1 VET" succeeds: MetaMask signs, status transitions `pending → mined(blk=N)`. Thor's stdout shows `eth-rpc method=eth_sendRawTransaction code=0`.
4. [ ] "Deploy Counter" succeeds: receipt.contractAddress matches the local `getCreateAddress(from, nonce)` computation. Green ✅ indicator.
5. [ ] `+1` and `+5` increment the counter; `value()` reads the updated value.
6. [ ] Right sidebar (⑤ RPC log) shows a scrolling list of every ethers.js RPC call with method name, params, result, and latency. Five random sampled entries align 1:1 with thor's stdout `eth-rpc` lines.

## Known limitations

- **MetaMask single-token model conflict**: MetaMask displays gas estimates in VET but Thor charges in VTHO. On solo this is cosmetic (dev accounts have unlimited VTHO). Mainnet deployment would require a separate VTHO-awareness layer. See spec §7 Risks.
- **eth_estimateGas latency**: the binary search in Thor's handler rebuilds state per iteration; MetaMask popups may feel sluggish. See review finding #5 in `docs/superpowers/eth-tx-summary.md`.

## Troubleshooting

- "MetaMask rejected": click the MetaMask icon — if VeChain Solo wasn't added, add it via the button; check that solo's `eth_chainId` matches what MetaMask was asked to add.
- `Address mismatch ❌`: verify your thor solo is running the `eth-nonce-create` branch or later (spec 3 must be implemented).
- No RPC log entries: the sidebar intercepts via a BrowserProvider subclass — if you see MetaMask popups but no sidebar entries, check the browser console for ethers v6 import failures.
