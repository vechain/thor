// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package filters

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

const (
	filterTTL        = 5 * time.Minute
	ttlCheckInterval = time.Minute
	pendingTxBufSize = 128

	// maxActiveFilters caps the total number of live filter objects across all clients.
	// Each kindPendingTx filter holds a live txpool subscription; without this cap a
	// single client can exhaust goroutines and channels by calling eth_newPendingTransactionFilter
	// in a tight loop. The TTL evicts idle filters after filterTTL, but the check interval is
	// ttlCheckInterval, so up to maxActiveFilters entries can accumulate before eviction fires.
	//
	// TODO: decide the broader approach for stateful filter endpoints:
	//   (a) keep as-is with this global cap + TTL and document that sticky sessions are required
	//       in multi-node / load-balanced deployments (filter state is node-local), or
	//   (b) add a node-operator flag to disable these endpoints for clustered setups.
	// Modern tooling (ethers v6, viem, wagmi) uses eth_subscribe over WebSocket instead;
	// these filter endpoints mainly serve legacy clients (web3.js v1, older Hardhat plugins).
	maxActiveFilters = 1000
)

type filterKind int8

const (
	kindLog filterKind = iota
	kindBlock
	kindPendingTx
)

type entry struct {
	kind     filterKind
	lastPoll time.Time
	// mu serialises concurrent ethGetFilterChanges calls on the same filter entry.
	// reader and txCh are stateful (capture position/buffer) and must not be read
	// by two goroutines at once. The TTL goroutine only holds h.mu, never e.mu.
	mu sync.Mutex

	// kindLog + kindBlock: tracks the chain cursor for incremental polling.
	// Positioned at the best block when the filter was created; advances on each poll.
	reader chain.BlockReader

	// kindLog only: the original filter object and its parsed criteria.
	//
	// eth_getFilterChanges uses criteria for fast per-event matching while
	// scanning new blocks via reader. It ignores LogFilter.FromBlock/ToBlock.
	//
	// eth_getFilterLogs re-evaluates LogFilter.FromBlock/ToBlock against the
	// current best chain at query time, so "latest" resolves to the current
	// head — not the block at filter creation.
	logFilter rpc.EthLogFilter
	criteria  ethconvert.LogCriteria

	// kindPendingTx only.
	// Only executable ETH-typed transactions are reported; see eth_newPendingTransactionFilter.
	txCh  chan *txpool.TxEvent
	txSub event.Subscription
}

// Handler implements the Ethereum filter poll API.
type Handler struct {
	repo      *chain.Repository
	txPool    txpool.Pool
	backtrace uint32

	mu      sync.Mutex
	entries map[string]*entry
	nextID  atomic.Uint64
	done    chan struct{}
	wg      sync.WaitGroup
}

// New creates a filter Handler and starts the background TTL cleanup goroutine.
func New(repo *chain.Repository, txPool txpool.Pool, backtrace uint32) *Handler {
	h := &Handler{
		repo:      repo,
		txPool:    txPool,
		backtrace: backtrace,
		entries:   make(map[string]*entry),
		done:      make(chan struct{}),
	}
	h.wg.Go(h.runTTL)
	return h
}

// Close stops the TTL goroutine and unsubscribes all pending-tx filter subscriptions.
func (h *Handler) Close() {
	close(h.done)
	h.wg.Wait()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range h.entries {
		if e.kind == kindPendingTx {
			e.txSub.Unsubscribe()
		}
	}
}

// Mount registers all filter methods on the dispatcher.
func (h *Handler) Mount(s *jsonrpc.Server) {
	s.Register("eth_newFilter", h.ethNewFilter)
	s.Register("eth_newBlockFilter", h.ethNewBlockFilter)
	s.Register("eth_newPendingTransactionFilter", h.ethNewPendingTransactionFilter)
	s.Register("eth_getFilterChanges", h.ethGetFilterChanges)
	s.Register("eth_getFilterLogs", h.ethGetFilterLogs)
	s.Register("eth_uninstallFilter", h.ethUninstallFilter)
}

func (h *Handler) newID() string {
	return hexutil.EncodeUint64(h.nextID.Add(1))
}

func (h *Handler) runTTL() {
	ticker := time.NewTicker(ttlCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.evictExpired()
		case <-h.done:
			return
		}
	}
}

func (h *Handler) evictExpired() {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for id, e := range h.entries {
		if now.Sub(e.lastPoll) > filterTTL {
			if e.kind == kindPendingTx {
				e.txSub.Unsubscribe()
			}
			delete(h.entries, id)
		}
	}
}

func (h *Handler) ethNewFilter(req jsonrpc.Request) jsonrpc.Response {
	var params []rpc.EthLogFilter
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "expected [filterObject]")
	}
	f := params[0]
	criteria, err := ethconvert.ParseLogCriteria(f)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, err.Error())
	}
	id := h.newID()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) >= maxActiveFilters {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeServerError, "too many active filters")
	}
	h.entries[id] = &entry{
		kind:      kindLog,
		lastPoll:  time.Now(),
		reader:    h.repo.NewBlockReader(h.repo.BestBlockSummary().Header.ID()),
		logFilter: f,
		criteria:  criteria,
	}
	return jsonrpc.OkResponse(req.ID, id)
}

func (h *Handler) ethNewBlockFilter(req jsonrpc.Request) jsonrpc.Response {
	id := h.newID()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) >= maxActiveFilters {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeServerError, "too many active filters")
	}
	h.entries[id] = &entry{
		kind:     kindBlock,
		lastPoll: time.Now(),
		reader:   h.repo.NewBlockReader(h.repo.BestBlockSummary().Header.ID()),
	}
	return jsonrpc.OkResponse(req.ID, id)
}

func (h *Handler) ethNewPendingTransactionFilter(req jsonrpc.Request) jsonrpc.Response {
	txCh := make(chan *txpool.TxEvent, pendingTxBufSize)
	sub := h.txPool.SubscribeTxEvent(txCh)
	id := h.newID()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) >= maxActiveFilters {
		sub.Unsubscribe()
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeServerError, "too many active filters")
	}
	h.entries[id] = &entry{
		kind:     kindPendingTx,
		lastPoll: time.Now(),
		txCh:     txCh,
		txSub:    sub,
	}
	return jsonrpc.OkResponse(req.ID, id)
}

func (h *Handler) ethGetFilterChanges(req jsonrpc.Request) jsonrpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "expected [filterId]")
	}
	var id string
	if err := json.Unmarshal(params[0], &id); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid filter id")
	}

	h.mu.Lock()
	e, ok := h.entries[id]
	if ok {
		e.lastPoll = time.Now()
	}
	h.mu.Unlock()

	if !ok {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "filter not found")
	}

	switch e.kind {
	case kindBlock:
		return h.changesBlock(req.ID, e)
	case kindLog:
		return h.changesLog(req.ID, e)
	default: // kindPendingTx
		return h.changesPendingTx(req.ID, e)
	}
}

func (h *Handler) changesBlock(id json.RawMessage, e *entry) jsonrpc.Response {
	e.mu.Lock()
	defer e.mu.Unlock()
	// BlockReader.Read() advances by one block per call — loop until caught up.
	hashes := make([]common.Hash, 0)
	for {
		blocks, err := e.reader.Read()
		if err != nil {
			return jsonrpc.ErrResponse(id, jsonrpc.CodeInternalError, err.Error())
		}
		if len(blocks) == 0 {
			break
		}
		for _, blk := range blocks {
			if blk.Obsolete {
				continue // skip fork/reorg blocks; only canonical new heads
			}
			hashes = append(hashes, common.Hash(blk.Header().ID()))
		}
	}
	return jsonrpc.OkResponse(id, hashes)
}

func (h *Handler) changesLog(id json.RawMessage, e *entry) jsonrpc.Response {
	e.mu.Lock()
	defer e.mu.Unlock()
	// BlockReader.Read() advances by one block per call — loop until caught up.
	ethLogs := make([]*rpc.EthLog, 0)
	for {
		blocks, err := e.reader.Read()
		if err != nil {
			return jsonrpc.ErrResponse(id, jsonrpc.CodeInternalError, err.Error())
		}
		if len(blocks) == 0 {
			break
		}
		for _, blk := range blocks {
			if blk.Obsolete {
				continue // skip fork/reorg blocks
			}
			receipts, err := h.repo.GetBlockReceipts(blk.Header().ID())
			if err != nil {
				return jsonrpc.ErrResponse(id, jsonrpc.CodeInternalError, err.Error())
			}
			logs := ethconvert.CollectMatchingLogs(&e.criteria, blk.Transactions(), receipts,
				common.Hash(blk.Header().ID()), uint64(blk.Header().Number()), false)
			ethLogs = append(ethLogs, logs...)
		}
	}
	return jsonrpc.OkResponse(id, ethLogs)
}

func (h *Handler) changesPendingTx(id json.RawMessage, e *entry) jsonrpc.Response {
	e.mu.Lock()
	defer e.mu.Unlock()
	var hashes []common.Hash
drain:
	for {
		select {
		case ev := <-e.txCh:
			if ev.Executable != nil && *ev.Executable && ev.Tx.Type() == tx.TypeEthDynamicFee {
				hashes = append(hashes, common.Hash(ev.Tx.ID()))
			}
		default:
			break drain
		}
	}
	if hashes == nil {
		hashes = []common.Hash{}
	}
	return jsonrpc.OkResponse(id, hashes)
}

func (h *Handler) ethGetFilterLogs(req jsonrpc.Request) jsonrpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "expected [filterId]")
	}
	var id string
	if err := json.Unmarshal(params[0], &id); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid filter id")
	}

	h.mu.Lock()
	e, ok := h.entries[id]
	if ok {
		e.lastPoll = time.Now()
	}
	h.mu.Unlock()

	if !ok {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "filter not found")
	}
	if e.kind != kindLog {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "eth_getFilterLogs is only valid for log filters")
	}
	return h.queryFilterLogs(req.ID, e)
}

// queryFilterLogs re-runs a full range query for a log filter using block receipt scanning.
//
// FromBlock/ToBlock from the stored LogFilter are re-resolved against the current best chain
// at call time: "latest" always means the current head, not the block at filter creation.
// Use eth_getFilterChanges for incremental changes from the creation cursor.
//
// Scanning is receipt-based rather than using the logDB index, so it is bounded by the
// backtrace limit. For large historical range queries, prefer eth_getLogs instead.
func (h *Handler) queryFilterLogs(id json.RawMessage, e *entry) jsonrpc.Response {
	f := e.logFilter
	bestNum := h.repo.BestBlockSummary().Header.Number()
	bestChain := h.repo.NewBestChain()

	// Default both fromBlock and toBlock to "latest" when absent.
	fromNum := bestNum
	toNum := bestNum

	if f.FromBlock != nil && *f.FromBlock != "" {
		summary, err := ethconvert.ResolveBlockTag(*f.FromBlock, h.repo)
		if err != nil {
			return jsonrpc.ErrResponse(id, jsonrpc.CodeInvalidParams, "invalid fromBlock")
		}
		fromNum = summary.Header.Number()
	}
	if f.ToBlock != nil && *f.ToBlock != "" {
		summary, err := ethconvert.ResolveBlockTag(*f.ToBlock, h.repo)
		if err != nil {
			return jsonrpc.ErrResponse(id, jsonrpc.CodeInvalidParams, "invalid toBlock")
		}
		toNum = summary.Header.Number()
	}
	if toNum > bestNum {
		toNum = bestNum
	}
	if fromNum > toNum {
		return jsonrpc.ErrResponse(id, jsonrpc.CodeInvalidParams, "invalid block range")
	}
	if toNum-fromNum > h.backtrace {
		return jsonrpc.ErrResponse(id, jsonrpc.CodeServerError,
			fmt.Sprintf("block range exceeds backtrace limit of %d", h.backtrace))
	}

	var ethLogs []*rpc.EthLog
	for num := uint64(fromNum); num <= uint64(toNum); num++ {
		blk, err := bestChain.GetBlock(uint32(num))
		if err != nil {
			return jsonrpc.ErrResponse(id, jsonrpc.CodeInternalError, err.Error())
		}
		receipts, err := h.repo.GetBlockReceipts(blk.Header().ID())
		if err != nil {
			return jsonrpc.ErrResponse(id, jsonrpc.CodeInternalError, err.Error())
		}
		logs := ethconvert.CollectMatchingLogs(&e.criteria, blk.Transactions(), receipts,
			common.Hash(blk.Header().ID()), uint64(blk.Header().Number()), false)
		ethLogs = append(ethLogs, logs...)
	}
	if ethLogs == nil {
		ethLogs = []*rpc.EthLog{}
	}
	return jsonrpc.OkResponse(id, ethLogs)
}

func (h *Handler) ethUninstallFilter(req jsonrpc.Request) jsonrpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "expected [filterId]")
	}
	var id string
	if err := json.Unmarshal(params[0], &id); err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid filter id")
	}

	h.mu.Lock()
	e, ok := h.entries[id]
	if ok {
		delete(h.entries, id)
	}
	h.mu.Unlock()

	if ok && e.kind == kindPendingTx {
		e.txSub.Unsubscribe()
	}
	return jsonrpc.OkResponse(req.ID, ok)
}
