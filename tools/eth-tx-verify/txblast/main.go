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
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// signerPoolStart / signerPoolEnd select the dev-account slice that txblast
// uses as tx origins. Genesis dev[0]/dev[1] are reserved (used by other
// tooling and the DApp's default account), so we pick uniformly at random
// from dev[3..9] — seven independent accounts give 0x02's per-account
// nonce counters enough headroom that REST and RPC submissions in the same
// batch only collide ~1/7 of the time, and that case is recovered by the
// per-address resync below.
const (
	signerPoolStart = 3
	signerPoolEnd   = 10 // exclusive
)

func main() {
	url := flag.String("url", "http://localhost:8669", "Solo node base URL")
	interval := flag.Duration("interval", 2*time.Second, "Batch interval")
	batch := flag.Int("batch", 1, "Multiplier per (type,path) cell (10 * batch = tx per tick)")
	receiptTimeout := flag.Duration("receipt-timeout", 15*time.Second, "Max age before a batch is printed with remaining MISSes (use ~1.5×block interval)")
	dryRun := flag.Bool("dry-run", false, "Build & sign but do not submit")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- Signer pool (dev[3..9]) ---
	// Each entry has its own EthNonce for sequential 0x02 nonces; 0x00/0x51
	// use random nonces and ignore the counter. Pulling 7 distinct accounts
	// also gives the 0x02 REST/RPC pair in each batch room to pick distinct
	// signers and avoid the same-signer-nonce race that previously forced
	// the dev[4]-only RPC path.
	pool, err := newSignerPool(ctx, *url)
	if err != nil {
		log.Fatalf("init signer pool: %v", err)
	}

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

	// --- Banner ---
	fmt.Printf("\ntxblast starting\n")
	fmt.Printf("  url=%s  interval=%s  batch=%d  dry-run=%t  receipt-timeout=%s\n",
		*url, *interval, *batch, *dryRun, *receiptTimeout)
	fmt.Printf("  signer pool (dev[%d..%d]):\n", signerPoolStart, signerPoolEnd-1)
	for i, e := range pool.entries {
		fmt.Printf("    dev[%d] %s  nonce=%d\n", signerPoolStart+i, e.addr.String(), e.nonce.Peek())
	}
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
					// Nonce recovery check tied to the head (oldest):
					// any pool account that produced a 0x02 nonce error in
					// this batch gets its counter resynced from chain.
					resyncDesynced(ctx, *url, pool, head.rows)
					pending = pending[1:]
				} else {
					break
				}
			}

			// 3. Submit new batch.
			batchN++
			rows := runBatch(ctx, *url, *batch, pool, chainTag, chainID, blockRef, *dryRun)
			pending = append(pending, &pendingBatch{batchN: batchN, submittedAt: time.Now(), rows: rows})

			// Refresh blockRef for newer tx validity.
			if ref, err := freshBlockRef(ctx, *url); err == nil {
				blockRef = ref
			}
		}
	}
}

// signerEntry pairs a dev account with its locally-tracked 0x02 nonce.
type signerEntry struct {
	key   *ecdsa.PrivateKey
	addr  thor.Address
	nonce *EthNonce
}

// signerPool is the fixed roster of dev[signerPoolStart..signerPoolEnd-1]
// used as transaction origins. Entries are immutable after construction;
// only the per-account EthNonce mutates.
type signerPool struct {
	entries []*signerEntry
	byAddr  map[thor.Address]*signerEntry
}

func newSignerPool(ctx context.Context, url string) (*signerPool, error) {
	accs := genesis.DevAccounts()[signerPoolStart:signerPoolEnd]
	p := &signerPool{
		entries: make([]*signerEntry, 0, len(accs)),
		byAddr:  make(map[thor.Address]*signerEntry, len(accs)),
	}
	for _, a := range accs {
		n, err := GetEthNonce(ctx, url, a.Address.String())
		if err != nil {
			return nil, fmt.Errorf("GetEthNonce %s: %w", a.Address.String(), err)
		}
		e := &signerEntry{key: a.PrivateKey, addr: a.Address, nonce: NewEthNonce(n)}
		p.entries = append(p.entries, e)
		p.byAddr[a.Address] = e
	}
	return p, nil
}

// random picks a uniform entry. The pool always has ≥2 entries (dev[3..9])
// so callers don't need to guard for the empty/single case.
func (p *signerPool) random() *signerEntry {
	return p.entries[rand.IntN(len(p.entries))]
}

// randomDistinct picks an entry different from excl. It loops on collision
// instead of building a filtered slice — pool size is 7 so the expected
// retry count is 1/7 ≈ 0.14.
func (p *signerPool) randomDistinct(excl *signerEntry) *signerEntry {
	for {
		s := p.random()
		if s != excl {
			return s
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

// resyncDesynced scans a finalized batch for 0x02 nonce errors and resyncs
// the on-chain nonce of every pool account that produced one. With seven
// independent accounts in the pool a single bad batch is the right
// granularity — there's no need for a streak counter to dampen on-demand
// solo's mining lag, since a desynced account otherwise blocks only its
// own future 0x02 submissions until the next collision triggers another
// resync.
func resyncDesynced(ctx context.Context, url string, pool *signerPool, rows []Row) {
	bad := map[thor.Address]struct{}{}
	for _, r := range rows {
		if r.Spec.Type != 0x02 || r.Submit.Err == nil {
			continue
		}
		msg := r.Submit.Err.Error()
		if !strings.Contains(msg, "nonce_too_low") && !strings.Contains(msg, "nonce_too_high") {
			continue
		}
		bad[r.Origin] = struct{}{}
	}
	for addr := range bad {
		entry, ok := pool.byAddr[addr]
		if !ok {
			continue
		}
		n, err := GetEthNonce(ctx, url, addr.String())
		if err != nil {
			continue
		}
		entry.nonce.Reset(n)
		fmt.Printf("[info] nonce resynced  %s → %d\n", addr.String(), n)
	}
}

func runBatch(ctx context.Context, url string, mult int,
	pool *signerPool,
	chainTag byte, chainID uint64, blockRef tx.BlockRef,
	dryRun bool,
) []Row {
	rows := make([]Row, 0, 10*mult)
	for range mult {
		// Pick distinct signers for the two 0x02 specs (REST + RPC) up front
		// so the same account doesn't appear twice in the same batch and
		// trigger the StrictlyAdd nonce-ahead reject on the second tx.
		// 0x00 / 0x51 use random nonces so they can share signers freely.
		eth02REST := pool.random()
		eth02RPC := pool.randomDistinct(eth02REST)
		for _, spec := range BuildMatrix() {
			var entry *signerEntry
			var txNonce uint64
			if spec.Type == 0x02 {
				if spec.Path == "rest" {
					entry = eth02REST
				} else {
					entry = eth02RPC
				}
				txNonce = entry.nonce.Take()
			} else {
				entry = pool.random()
			}
			trx, err := Build(spec, entry.key, txNonce, chainTag, chainID, blockRef)
			if err != nil {
				rows = append(rows, Row{Spec: spec, Origin: entry.addr, Submit: SubmitResult{Err: err}})
				continue
			}
			raw, err := trx.MarshalBinary()
			if err != nil {
				rows = append(rows, Row{Spec: spec, Origin: entry.addr, Submit: SubmitResult{Err: err}})
				continue
			}
			if dryRun {
				fmt.Printf("DRY-RUN %s/%d/%s from=%s raw=0x%x\n",
					formatType(spec.Type), spec.Clauses, spec.Path, entry.addr.String(), raw)
				rows = append(rows, Row{Spec: spec, Origin: entry.addr, Submit: SubmitResult{TxID: "dry-run"}})
				continue
			}
			var sub SubmitResult
			if spec.Path == "rest" {
				sub = SubmitREST(ctx, url, raw)
			} else {
				sub = SubmitRPC(ctx, url, raw)
			}
			rows = append(rows, Row{Spec: spec, Origin: entry.addr, Submit: sub})
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
	req.Header.Set("Referer", refererValue)
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
