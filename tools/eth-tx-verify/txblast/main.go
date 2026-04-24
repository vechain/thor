// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// txblast sends VeChain-native and Ethereum-style transactions against a solo
// node to verify the spec-1/2/3 implementation. See docs/superpowers/specs/
// 2026-04-24-eth-tx-verification-toolkit-design.md §3.
package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Solo dev account #0 — infinite VET + VTHO. Used as the default for all
// tx types except 0x02 RPC (which needs its own signer to avoid a nonce
// race when the REST path submits in the same batch; see the 0x02-RPC
// signer note below).
const defaultSoloDevKey = "99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36"

func main() {
	url := flag.String("url", "http://localhost:8669", "Solo node base URL")
	interval := flag.Duration("interval", 2*time.Second, "Batch interval")
	batch := flag.Int("batch", 1, "Multiplier per (type,path) cell (10 * batch = tx per tick)")
	keyFlag := flag.String("key", defaultSoloDevKey, "Hex private key for REST + VET paths (no 0x prefix)")
	receiptTimeout := flag.Duration("receipt-timeout", 15*time.Second, "Max age before a batch is printed with remaining MISSes (use ~1.5×block interval)")
	dryRun := flag.Bool("dry-run", false, "Build & sign but do not submit")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- Primary signer (REST 0x02 + all 0x00/0x51) ---
	key, err := crypto.HexToECDSA(*keyFlag)
	if err != nil {
		log.Fatalf("bad -key: %v", err)
	}
	signerAddr := crypto.PubkeyToAddress(key.PublicKey).Hex()

	// --- Secondary signer for 0x02 RPC path (dev account #4) ---
	// Rationale: within one batch we submit 0x02 on REST and then 0x02 on
	// RPC back-to-back. Both hit txpool.StrictlyAdd which rejects any tx
	// whose nonce is ahead of the account's state nonce ("tx is not
	// executable"). Using the same signer for both forces the second
	// submission to wait for the first to be mined and for state to
	// advance — a race that's flaky in timer-based solo and even in
	// on-demand solo under load. Separating signers gives each path its
	// own independent nonce counter and removes the dependency entirely.
	keyRPC := genesis.DevAccounts()[4].PrivateKey
	signerRPCAddr := crypto.PubkeyToAddress(keyRPC.PublicKey).Hex()

	// --- ChainID / chainTag / blockRef ---
	chainID, err := GetEthChainID(ctx, *url)
	if err != nil {
		log.Fatalf("GetEthChainID: %v", err)
	}
	chainTag, err := fetchGenesisChainTag(ctx, *url)
	if err != nil {
		log.Fatalf("fetchGenesisChainTag: %v", err)
	}
	blockRef, err := freshBlockRef(ctx, *url)
	if err != nil {
		log.Fatalf("freshBlockRef: %v", err)
	}

	// --- Nonce counters: one per 0x02 signer ---
	initialNonce, err := GetEthNonce(ctx, *url, signerAddr)
	if err != nil {
		log.Fatalf("GetEthNonce primary: %v", err)
	}
	nonce := NewEthNonce(initialNonce)

	initialNonceRPC, err := GetEthNonce(ctx, *url, signerRPCAddr)
	if err != nil {
		log.Fatalf("GetEthNonce rpc: %v", err)
	}
	nonceRPC := NewEthNonce(initialNonceRPC)

	// --- Banner ---
	fmt.Printf("\ntxblast starting\n")
	fmt.Printf("  url=%s  interval=%s  batch=%d  dry-run=%t  receipt-timeout=%s\n",
		*url, *interval, *batch, *dryRun, *receiptTimeout)
	fmt.Printf("  signer (REST + VET)     = %s  nonce=%d\n", signerAddr, initialNonce)
	fmt.Printf("  signer (RPC 0x02)       = %s  nonce=%d\n", signerRPCAddr, initialNonceRPC)
	fmt.Printf("  chainID=%d (0x%x)  chainTag=0x%02x  blockRef=0x%x\n\n",
		chainID, chainID, chainTag, blockRef)

	// --- Batch loop with pending queue ---
	// Each tick:
	//   1. Query receipts for every unresolved row in every pending batch.
	//   2. While the oldest pending batch is (a) fully resolved or
	//      (b) older than receiptTimeout, print it and pop.
	//   3. Submit a new batch; push to the queue tail.
	//
	// This decouples "when a batch was submitted" from "when its receipts
	// are available" so timer-based solo (10s block interval) doesn't
	// leave rows stuck on MISS forever.
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	var pending []*pendingBatch
	batchN := 0
	var nonceDesyncStreak, nonceRPCDesyncStreak int

	flush := func() {
		// Called on shutdown — drain whatever's left with one last
		// receipt pass and print.
		for _, pb := range pending {
			queryPendingReceipts(ctx, *url, pb.rows)
			printBatch(pb)
		}
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			fmt.Println("txblast stopping")
			return

		case <-ticker.C:
			// 1. Query pending receipts.
			for _, pb := range pending {
				queryPendingReceipts(ctx, *url, pb.rows)
			}

			// 2. Pop & print resolved or aged-out batches (at most one
			//    per tick keeps the stream readable).
			for len(pending) > 0 {
				head := pending[0]
				if allResolved(head.rows) || time.Since(head.submittedAt) >= *receiptTimeout {
					printBatch(head)
					// Nonce recovery check tied to the head (which is oldest).
					nonceDesyncStreak, nonceRPCDesyncStreak = updateDesyncAndMaybeReset(
						ctx, *url, head.rows, nonce, nonceRPC,
						signerAddr, signerRPCAddr, nonceDesyncStreak, nonceRPCDesyncStreak,
					)
					pending = pending[1:]
				} else {
					break
				}
			}

			// 3. Submit new batch.
			batchN++
			rows := runBatch(ctx, *url, *batch, key, keyRPC, chainTag, chainID, blockRef, nonce, nonceRPC, *dryRun)
			pending = append(pending, &pendingBatch{batchN: batchN, submittedAt: time.Now(), rows: rows})

			// Refresh blockRef for newer tx validity.
			if ref, err := freshBlockRef(ctx, *url); err == nil {
				blockRef = ref
			}
		}
	}
}

// pendingBatch tracks a submitted batch awaiting receipt resolution.
type pendingBatch struct {
	batchN      int
	submittedAt time.Time
	rows        []Row
}

// allResolved reports whether every row in rows has reached a terminal state
// (submit error, dry-run, or a receipt with Found=true or non-nil Err).
func allResolved(rows []Row) bool {
	for _, r := range rows {
		if r.Submit.Err != nil || r.Submit.TxID == "" || r.Submit.TxID == "dry-run" {
			continue // terminal
		}
		if !r.Receipt.Found && r.Receipt.Err == nil {
			return false // still pending
		}
	}
	return true
}

func printBatch(pb *pendingBatch) {
	PrintBatchHeader(pb.batchN, pb.submittedAt)
	for _, r := range pb.rows {
		PrintRow(r)
	}
	PrintSummary(pb.rows)
}

// updateDesyncAndMaybeReset is called when a batch's receipts are finalized
// (either fully resolved or aged out). It tracks consecutive desync streaks
// per 0x02 signer and resyncs the nonce counter after three bad batches in a
// row. Returns the updated streak counters.
func updateDesyncAndMaybeReset(ctx context.Context, url string, rows []Row,
	nonce, nonceRPC *EthNonce, primaryAddr, rpcAddr string,
	primaryStreak, rpcStreak int,
) (int, int) {
	priDesync, rpcDesync := 0, 0
	for _, r := range rows {
		if r.Spec.Type != 0x02 || r.Submit.Err == nil {
			continue
		}
		msg := r.Submit.Err.Error()
		if !strings.Contains(msg, "nonce_too_low") && !strings.Contains(msg, "nonce_too_high") {
			continue
		}
		if r.Spec.Path == "rest" {
			priDesync++
		} else {
			rpcDesync++
		}
	}

	if priDesync > 0 {
		primaryStreak++
	} else {
		primaryStreak = 0
	}
	if rpcDesync > 0 {
		rpcStreak++
	} else {
		rpcStreak = 0
	}

	if primaryStreak >= 3 {
		if n, err := GetEthNonce(ctx, url, primaryAddr); err == nil {
			nonce.Reset(n)
			fmt.Printf("[info] primary nonce resynced to %d\n", n)
		}
		primaryStreak = 0
	}
	if rpcStreak >= 3 {
		if n, err := GetEthNonce(ctx, url, rpcAddr); err == nil {
			nonceRPC.Reset(n)
			fmt.Printf("[info] RPC-signer nonce resynced to %d\n", n)
		}
		rpcStreak = 0
	}

	return primaryStreak, rpcStreak
}

func runBatch(ctx context.Context, url string, mult int,
	key, keyRPC *ecdsa.PrivateKey,
	chainTag byte, chainID uint64, blockRef tx.BlockRef,
	nonce, nonceRPC *EthNonce, dryRun bool,
) []Row {
	rows := make([]Row, 0, 10*mult)
	for range mult {
		for _, spec := range BuildMatrix() {
			// Pick signer + nonce based on (type, path).
			// 0x02 RPC uses keyRPC to avoid the same-signer-nonce race.
			signingKey := key
			var txNonce uint64
			if spec.Type == 0x02 {
				if spec.Path == "rpc" {
					signingKey = keyRPC
					txNonce = nonceRPC.Take()
				} else {
					txNonce = nonce.Take()
				}
			}
			trx, err := Build(spec, signingKey, txNonce, chainTag, chainID, blockRef)
			if err != nil {
				rows = append(rows, Row{Spec: spec, Submit: SubmitResult{Err: err}})
				continue
			}
			raw, err := trx.MarshalBinary()
			if err != nil {
				rows = append(rows, Row{Spec: spec, Submit: SubmitResult{Err: err}})
				continue
			}
			if dryRun {
				fmt.Printf("DRY-RUN %s/%d/%s raw=0x%x\n", formatType(spec.Type), spec.Clauses, spec.Path, raw)
				rows = append(rows, Row{Spec: spec, Submit: SubmitResult{TxID: "dry-run"}})
				continue
			}
			var sub SubmitResult
			if spec.Path == "rest" {
				sub = SubmitREST(ctx, url, raw)
			} else {
				sub = SubmitRPC(ctx, url, raw)
			}
			rows = append(rows, Row{Spec: spec, Submit: sub})
		}
	}
	return rows
}

// queryPendingReceipts updates the Receipt field for every row in rows that
// has a txid but hasn't yet resolved. Rows in a terminal state (submit error,
// dry-run, already found, already errored) are skipped so we don't retry
// indefinitely.
func queryPendingReceipts(ctx context.Context, url string, rows []Row) {
	for i := range rows {
		r := &rows[i]
		if r.Submit.Err != nil || r.Submit.TxID == "" || r.Submit.TxID == "dry-run" {
			continue
		}
		if r.Receipt.Found || r.Receipt.Err != nil {
			continue
		}
		if r.Spec.Path == "rest" {
			r.Receipt = ReceiptREST(ctx, url, r.Submit.TxID)
		} else {
			r.Receipt = ReceiptRPC(ctx, url, r.Submit.TxID)
		}
	}
}

// fetchGenesisChainTag returns the last byte of /blocks/0 ID.
func fetchGenesisChainTag(ctx context.Context, base string) (byte, error) {
	id, err := fetchBlockID(ctx, base, "0")
	if err != nil {
		return 0, err
	}
	return id[31], nil
}

// freshBlockRef returns the first 8 bytes of /blocks/best ID as a BlockRef.
func freshBlockRef(ctx context.Context, base string) (tx.BlockRef, error) {
	id, err := fetchBlockID(ctx, base, "best")
	if err != nil {
		return tx.BlockRef{}, err
	}
	var ref tx.BlockRef
	copy(ref[:], id[:8])
	return ref, nil
}

func fetchBlockID(ctx context.Context, base, tag string) (thor.Bytes32, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/blocks/"+tag, nil)
	if err != nil {
		return thor.Bytes32{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return thor.Bytes32{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return thor.Bytes32{}, fmt.Errorf("GET /blocks/%s: %d %s", tag, resp.StatusCode, string(b))
	}
	var blk struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &blk); err != nil {
		return thor.Bytes32{}, err
	}
	return thor.ParseBytes32(blk.ID)
}

// formatType renders the tx type byte as "0x02" / "0x00" / "0x51".
func formatType(b byte) string { return fmt.Sprintf("0x%02x", b) }
