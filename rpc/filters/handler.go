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
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

const (
	filterTTL        = 5 * time.Minute
	ttlCheckInterval = time.Minute
	pendingTxBufSize = 128
)

type filterKind int8

const (
	kindLog filterKind = iota
	kindBlock
	kindPendingTx
)

// logCriteria is the parsed form of a log filter for fast per-event matching
// during incremental block scanning in eth_getFilterChanges.
// Only ETH-typed (TypeEthDynamicFee) transaction events are matched.
type logCriteria struct {
	addresses []thor.Address
	topics    [5]*thor.Bytes32
}

func (c *logCriteria) matchesEvent(e *tx.Event) bool {
	if len(c.addresses) > 0 {
		found := false
		for _, a := range c.addresses {
			if a == e.Address {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for i, want := range c.topics {
		if want == nil {
			continue // wildcard
		}
		if i >= len(e.Topics) || e.Topics[i] != *want {
			return false
		}
	}
	return true
}

type entry struct {
	kind     filterKind
	lastPoll time.Time

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
	logFilter rpc.LogFilter
	criteria  logCriteria

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
func (h *Handler) Mount(s *rpc.Server) {
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

func (h *Handler) ethNewFilter(req rpc.Request) rpc.Response {
	var params []rpc.LogFilter
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [filterObject]")
	}
	f := params[0]
	criteria, err := parseCriteria(f)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, err.Error())
	}
	id := h.newID()
	h.mu.Lock()
	h.entries[id] = &entry{
		kind:      kindLog,
		lastPoll:  time.Now(),
		reader:    h.repo.NewBlockReader(h.repo.BestBlockSummary().Header.ID()),
		logFilter: f,
		criteria:  criteria,
	}
	h.mu.Unlock()
	return rpc.OkResponse(req.ID, id)
}

func (h *Handler) ethNewBlockFilter(req rpc.Request) rpc.Response {
	id := h.newID()
	h.mu.Lock()
	h.entries[id] = &entry{
		kind:     kindBlock,
		lastPoll: time.Now(),
		reader:   h.repo.NewBlockReader(h.repo.BestBlockSummary().Header.ID()),
	}
	h.mu.Unlock()
	return rpc.OkResponse(req.ID, id)
}

func (h *Handler) ethNewPendingTransactionFilter(req rpc.Request) rpc.Response {
	txCh := make(chan *txpool.TxEvent, pendingTxBufSize)
	sub := h.txPool.SubscribeTxEvent(txCh)
	id := h.newID()
	h.mu.Lock()
	h.entries[id] = &entry{
		kind:     kindPendingTx,
		lastPoll: time.Now(),
		txCh:     txCh,
		txSub:    sub,
	}
	h.mu.Unlock()
	return rpc.OkResponse(req.ID, id)
}

func (h *Handler) ethGetFilterChanges(req rpc.Request) rpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [filterId]")
	}
	var id string
	if err := json.Unmarshal(params[0], &id); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid filter id")
	}

	h.mu.Lock()
	e, ok := h.entries[id]
	if ok {
		e.lastPoll = time.Now()
	}
	h.mu.Unlock()

	if !ok {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "filter not found")
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

func (h *Handler) changesBlock(id json.RawMessage, e *entry) rpc.Response {
	// BlockReader.Read() advances by one block per call — loop until caught up.
	hashes := make([]common.Hash, 0)
	for {
		blocks, err := e.reader.Read()
		if err != nil {
			return rpc.ErrResponse(id, rpc.CodeInternalError, err.Error())
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
	return rpc.OkResponse(id, hashes)
}

func (h *Handler) changesLog(id json.RawMessage, e *entry) rpc.Response {
	// BlockReader.Read() advances by one block per call — loop until caught up.
	ethLogs := make([]*rpc.EthLog, 0)
	for {
		blocks, err := e.reader.Read()
		if err != nil {
			return rpc.ErrResponse(id, rpc.CodeInternalError, err.Error())
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
				return rpc.ErrResponse(id, rpc.CodeInternalError, err.Error())
			}
			logs := collectMatchingLogs(&e.criteria, blk.Transactions(), receipts,
				common.Hash(blk.Header().ID()), uint64(blk.Header().Number()))
			ethLogs = append(ethLogs, logs...)
		}
	}
	return rpc.OkResponse(id, ethLogs)
}

func (h *Handler) changesPendingTx(id json.RawMessage, e *entry) rpc.Response {
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
	return rpc.OkResponse(id, hashes)
}

func (h *Handler) ethGetFilterLogs(req rpc.Request) rpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [filterId]")
	}
	var id string
	if err := json.Unmarshal(params[0], &id); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid filter id")
	}

	h.mu.Lock()
	e, ok := h.entries[id]
	if ok {
		e.lastPoll = time.Now()
	}
	h.mu.Unlock()

	if !ok {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "filter not found")
	}
	if e.kind != kindLog {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "eth_getFilterLogs is only valid for log filters")
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
func (h *Handler) queryFilterLogs(id json.RawMessage, e *entry) rpc.Response {
	f := e.logFilter
	bestNum := h.repo.BestBlockSummary().Header.Number()
	bestChain := h.repo.NewBestChain()

	// Default both fromBlock and toBlock to "latest" when absent.
	fromNum := bestNum
	toNum := bestNum

	if f.FromBlock != nil && *f.FromBlock != "" {
		summary, err := rpc.ResolveBlockTag(*f.FromBlock, h.repo)
		if err != nil {
			return rpc.ErrResponse(id, rpc.CodeInvalidParams, "invalid fromBlock")
		}
		fromNum = summary.Header.Number()
	}
	if f.ToBlock != nil && *f.ToBlock != "" {
		summary, err := rpc.ResolveBlockTag(*f.ToBlock, h.repo)
		if err != nil {
			return rpc.ErrResponse(id, rpc.CodeInvalidParams, "invalid toBlock")
		}
		toNum = summary.Header.Number()
	}
	if toNum > bestNum {
		toNum = bestNum
	}
	if fromNum > toNum {
		return rpc.ErrResponse(id, rpc.CodeInvalidParams, "invalid block range")
	}
	if toNum-fromNum > h.backtrace {
		return rpc.ErrResponse(id, rpc.CodeServerError,
			fmt.Sprintf("block range exceeds backtrace limit of %d", h.backtrace))
	}

	var ethLogs []*rpc.EthLog
	for num := uint64(fromNum); num <= uint64(toNum); num++ {
		blk, err := bestChain.GetBlock(uint32(num))
		if err != nil {
			return rpc.ErrResponse(id, rpc.CodeInternalError, err.Error())
		}
		receipts, err := h.repo.GetBlockReceipts(blk.Header().ID())
		if err != nil {
			return rpc.ErrResponse(id, rpc.CodeInternalError, err.Error())
		}
		logs := collectMatchingLogs(&e.criteria, blk.Transactions(), receipts,
			common.Hash(blk.Header().ID()), uint64(blk.Header().Number()))
		ethLogs = append(ethLogs, logs...)
	}
	if ethLogs == nil {
		ethLogs = []*rpc.EthLog{}
	}
	return rpc.OkResponse(id, ethLogs)
}

func (h *Handler) ethUninstallFilter(req rpc.Request) rpc.Response {
	var params [1]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [filterId]")
	}
	var id string
	if err := json.Unmarshal(params[0], &id); err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid filter id")
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
	return rpc.OkResponse(req.ID, ok)
}

// collectMatchingLogs scans ETH-typed transactions in a single block and returns EthLog
// entries matching the criteria. Projected transactionIndex and logIndex are relative to
// ETH-typed transactions only, consistent with eth_getTransactionByHash etc.
func collectMatchingLogs(criteria *logCriteria, txs tx.Transactions, receipts tx.Receipts, blockHash common.Hash, blockNum uint64) []*rpc.EthLog {
	var logs []*rpc.EthLog
	var projEthIdx uint64 // running ETH tx index within the block
	var projLogIdx uint64 // running ETH log index within the block (all ETH events, not just matching)

	for i, t := range txs {
		if t.Type() != tx.TypeEthDynamicFee {
			continue
		}
		receipt := receipts[i]
		if len(receipt.Outputs) > 0 {
			for j, event := range receipt.Outputs[0].Events {
				if criteria.matchesEvent(event) {
					topics := make([]common.Hash, len(event.Topics))
					for k, tp := range event.Topics {
						topics[k] = common.Hash(tp)
					}
					logs = append(logs, &rpc.EthLog{
						Address:     common.Address(event.Address),
						Topics:      topics,
						Data:        event.Data,
						BlockNumber: hexutil.Uint64(blockNum),
						TxHash:      common.Hash(t.ID()),
						TxIndex:     hexutil.Uint64(projEthIdx),
						BlockHash:   blockHash,
						LogIndex:    hexutil.Uint64(projLogIdx + uint64(j)),
						Removed:     false,
					})
				}
			}
			projLogIdx += uint64(len(receipt.Outputs[0].Events))
		}
		projEthIdx++
	}
	return logs
}

// parseCriteria parses the address and topic fields from a LogFilter into a logCriteria.
// OR semantics within a single topic position are not fully supported — only the first
// alternative is used (e.g., [["A","B"], "C"] treats position 0 as matching only "A").
func parseCriteria(f rpc.LogFilter) (logCriteria, error) {
	var c logCriteria

	if len(f.Address) > 0 && string(f.Address) != "null" {
		var single string
		var multi []string
		if err := json.Unmarshal(f.Address, &single); err == nil {
			addr, err := thor.ParseAddress(single)
			if err != nil {
				return c, fmt.Errorf("invalid address: %w", err)
			}
			c.addresses = append(c.addresses, addr)
		} else if err := json.Unmarshal(f.Address, &multi); err == nil {
			for _, s := range multi {
				addr, err := thor.ParseAddress(s)
				if err != nil {
					return c, fmt.Errorf("invalid address: %w", err)
				}
				c.addresses = append(c.addresses, addr)
			}
		}
	}

	topics := f.Topics
	if len(topics) > len(c.topics) {
		topics = topics[:len(c.topics)]
	}
	for i, raw := range topics {
		if raw == nil || string(raw) == "null" {
			continue
		}
		var single string
		var multi []string
		if err := json.Unmarshal(raw, &single); err == nil {
			h32, err := rpc.ParseBytes32Compact(single)
			if err != nil {
				return c, fmt.Errorf("invalid topic: %w", err)
			}
			h32Copy := h32
			c.topics[i] = &h32Copy
		} else if err := json.Unmarshal(raw, &multi); err == nil && len(multi) > 0 {
			h32, err := rpc.ParseBytes32Compact(multi[0])
			if err != nil {
				return c, fmt.Errorf("invalid topic: %w", err)
			}
			h32Copy := h32
			c.topics[i] = &h32Copy
		}
	}
	return c, nil
}
