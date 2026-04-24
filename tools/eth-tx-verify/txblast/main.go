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
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Solo dev account #0 — infinite VET + VTHO. See
// thor/genesis/devnet.go (or equivalent) for the canonical list.
const defaultSoloDevKey = "99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36"

func main() {
	url := flag.String("url", "http://localhost:8669", "Solo node base URL")
	interval := flag.Duration("interval", 2*time.Second, "Batch interval")
	batch := flag.Int("batch", 1, "Multiplier per (type,path) cell (10 * batch = tx per tick)")
	keyFlag := flag.String("key", defaultSoloDevKey, "Hex private key (no 0x prefix)")
	dryRun := flag.Bool("dry-run", false, "Build & sign but do not submit")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- Derive signer address + private key ---
	key, err := crypto.HexToECDSA(*keyFlag)
	if err != nil {
		log.Fatalf("bad -key: %v", err)
	}
	ethAddr := crypto.PubkeyToAddress(key.PublicKey)
	signerAddr := ethAddr.Hex()
	signer := thor.BytesToAddress(ethAddr.Bytes())
	_ = signer // may be useful for future diagnostics

	// --- Fetch chainID ---
	chainID, err := GetEthChainID(ctx, *url)
	if err != nil {
		log.Fatalf("GetEthChainID: %v", err)
	}
	log.Printf("chainID: %d (0x%x)", chainID, chainID)

	// --- Fetch genesis chainTag (last byte of /blocks/0 ID) ---
	chainTag, err := fetchGenesisChainTag(ctx, *url)
	if err != nil {
		log.Fatalf("fetchGenesisChainTag: %v", err)
	}
	log.Printf("chainTag: 0x%02x", chainTag)

	// --- Fetch best block blockRef ---
	blockRef, err := freshBlockRef(ctx, *url)
	if err != nil {
		log.Fatalf("freshBlockRef: %v", err)
	}
	log.Printf("initial blockRef: 0x%x", blockRef)

	// --- Fetch 0x02 nonce ---
	initialNonce, err := GetEthNonce(ctx, *url, signerAddr)
	if err != nil {
		log.Fatalf("GetEthNonce: %v", err)
	}
	nonce := NewEthNonce(initialNonce)
	log.Printf("initial nonce: %d", initialNonce)

	// --- Start banner ---
	fmt.Printf("\ntxblast starting\n")
	fmt.Printf("  url=%s  interval=%s  batch=%d  dry-run=%t\n", *url, *interval, *batch, *dryRun)
	fmt.Printf("  signer=%s\n", signerAddr)
	fmt.Printf("  chainID=%d  chainTag=0x%02x  blockRef=0x%x  nonce=%d\n\n",
		chainID, chainTag, blockRef, initialNonce)

	// --- Batch loop ---
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	var lastBatch []Row
	batchN := 0
	var nonceDesyncStreak int

	for {
		select {
		case <-ctx.Done():
			// Final tick — query receipts for the last batch and print.
			if len(lastBatch) > 0 {
				queryReceipts(ctx, *url, lastBatch)
				PrintBatchHeader(batchN, time.Now())
				for _, r := range lastBatch {
					PrintRow(r)
				}
				PrintSummary(lastBatch)
			}
			fmt.Println("txblast stopping")
			return

		case <-ticker.C:
			// Query receipts for the PREVIOUS batch, print it.
			if len(lastBatch) > 0 {
				queryReceipts(ctx, *url, lastBatch)
				PrintBatchHeader(batchN, time.Now())
				for _, r := range lastBatch {
					PrintRow(r)
				}
				PrintSummary(lastBatch)

				// Nonce desync recovery.
				desyncThisBatch := 0
				for _, r := range lastBatch {
					if r.Spec.Type == 0x02 && r.Submit.Err != nil &&
						(strings.Contains(r.Submit.Err.Error(), "nonce_too_low") ||
							strings.Contains(r.Submit.Err.Error(), "nonce_too_high")) {
						desyncThisBatch++
					}
				}
				if desyncThisBatch > 0 {
					nonceDesyncStreak++
				} else {
					nonceDesyncStreak = 0
				}
				if nonceDesyncStreak >= 3 {
					n, err := GetEthNonce(ctx, *url, signerAddr)
					if err == nil {
						nonce.Reset(n)
						fmt.Println("[info] nonce resynced to", n)
					}
					nonceDesyncStreak = 0
				}
			}

			// Build + submit the NEW batch.
			batchN++
			lastBatch = runBatch(ctx, *url, *batch, key, chainTag, chainID, blockRef, nonce, *dryRun)

			// Refresh blockRef for newer tx validity.
			if ref, err := freshBlockRef(ctx, *url); err == nil {
				blockRef = ref
			}
		}
	}
}

func runBatch(ctx context.Context, url string, mult int, key *ecdsa.PrivateKey,
	chainTag byte, chainID uint64, blockRef tx.BlockRef, nonce *EthNonce, dryRun bool) []Row {

	rows := make([]Row, 0, 10*mult)
	for range mult {
		for _, spec := range BuildMatrix() {
			var txNonce uint64
			if spec.Type == 0x02 {
				txNonce = nonce.Take()
			}
			trx, err := Build(spec, key, txNonce, chainTag, chainID, blockRef)
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

func queryReceipts(ctx context.Context, url string, rows []Row) {
	for i := range rows {
		if rows[i].Submit.Err != nil || rows[i].Submit.TxID == "" || rows[i].Submit.TxID == "dry-run" {
			continue
		}
		if rows[i].Spec.Path == "rest" {
			rows[i].Receipt = ReceiptREST(ctx, url, rows[i].Submit.TxID)
		} else {
			rows[i].Receipt = ReceiptRPC(ctx, url, rows[i].Submit.TxID)
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
