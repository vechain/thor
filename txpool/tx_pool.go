// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"container/heap"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
	// max size of tx allowed
	MaxTxSize = 64 * 1024
)

var logger = log.WithContext("pkg", "txpool")

// Options options for tx pool.
type Options struct {
	Limit                  int
	LimitPerAccount        int
	MaxLifetime            time.Duration
	BlocklistCacheFilePath string
	BlocklistFetchURL      string
}

// TxEvent will be posted when tx is added or status changed.
type TxEvent struct {
	Tx         *tx.Transaction
	Executable *bool
}

// Pool defines the interface for the transaction pool.
type Pool interface {
	Get(txID thor.Bytes32) *tx.Transaction
	Add(newTx *tx.Transaction) error
	AddLocal(tx *tx.Transaction) error
	StrictlyAdd(newTx *tx.Transaction) error
	Remove(txHash thor.Bytes32, txID thor.Bytes32) bool
	Dump() tx.Transactions
	Len() int
	SubscribeTxEvent(chan *TxEvent) event.Subscription
	Executables() tx.Transactions
	Fill(txs tx.Transactions)
	Close()
	PoolNonce(addr thor.Address) uint64
	GetByHash(hash thor.Bytes32) *tx.Transaction
}

// TxPool is the top-level Pool implementation.  It owns the two
// family-specific sub-pools (VeChainPool, EthPool), routes incoming txs by
// family, unifies event feeds, and performs the cross-family EFS merge that
// produces the single ordered list consumed by the packer.
//
// External contract:
//   - Implements Pool (including the new PoolNonce / GetByHash methods).
//   - Events from either sub-pool are re-emitted on a single unified feed so
//     consumers (subscriptions, comm) subscribe once.
//   - Executables() guarantees EthereumTx nonce-order preservation per sender
//     while still globally ordering by priorityGasPrice descending.
type TxPool struct {
	vechain *VeChainPool
	eth     *EthPool

	txFeed event.Feed
	scope  event.SubscriptionScope

	ctx    chan struct{}
	goes   sync.WaitGroup
	closed bool
	mu     sync.Mutex
}

// New constructs a TxPool with VeChain and Ethereum sub-pools.
func New(
	repo *chain.Repository,
	stater *state.Stater,
	options Options,
	forkConfig *thor.ForkConfig,
) *TxPool {
	vechain := newVeChainPool(repo, stater, options, forkConfig)
	eth := NewEth(repo, stater, forkConfig)
	c := &TxPool{
		vechain: vechain,
		eth:     eth,
		ctx:     make(chan struct{}),
	}
	c.startEventRelay()
	return c
}

// VeChain returns the underlying VeChainPool.  Test-only accessor.
func (c *TxPool) VeChain() *VeChainPool { return c.vechain }

// Eth returns the underlying EthPool.  Test-only accessor.
func (c *TxPool) Eth() *EthPool { return c.eth }

// startEventRelay spawns two goroutines that forward sub-pool events onto the
// coordinator's unified feed.  Buffer size 32 matches typical event.Feed usage.
func (c *TxPool) startEventRelay() {
	relay := func(sub func(chan *TxEvent) event.Subscription) {
		ch := make(chan *TxEvent, 32)
		s := sub(ch)
		c.goes.Go(func() {
			defer s.Unsubscribe()
			for {
				select {
				case <-c.ctx:
					return
				case ev := <-ch:
					if ev == nil {
						return
					}
					c.txFeed.Send(ev)
				}
			}
		})
	}
	relay(c.vechain.SubscribeTxEvent)
	relay(c.eth.SubscribeTxEvent)
}

// Close stops the relay goroutines and closes both sub-pools.
func (c *TxPool) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.ctx)
	c.mu.Unlock()

	c.scope.Close()
	c.vechain.Close()
	c.eth.Close()
	c.goes.Wait()
}

// SubscribeTxEvent subscribes on the unified feed.
func (c *TxPool) SubscribeTxEvent(ch chan *TxEvent) event.Subscription {
	return c.scope.Track(c.txFeed.Subscribe(ch))
}

// route selects the sub-pool for newTx.
func (c *TxPool) route(newTx *tx.Transaction) Pool {
	if newTx.IsEthereumTx() {
		return c.eth
	}
	return c.vechain
}

// Add admits newTx into its family's sub-pool.
func (c *TxPool) Add(newTx *tx.Transaction) error {
	return c.route(newTx).Add(newTx)
}

// AddLocal admits newTx as locally submitted into its family's sub-pool.
func (c *TxPool) AddLocal(newTx *tx.Transaction) error {
	return c.route(newTx).AddLocal(newTx)
}

// StrictlyAdd requires newTx to be immediately executable.
func (c *TxPool) StrictlyAdd(newTx *tx.Transaction) error {
	return c.route(newTx).StrictlyAdd(newTx)
}

// Get looks up a tx in either sub-pool by ID.
func (c *TxPool) Get(id thor.Bytes32) *tx.Transaction {
	if t := c.vechain.Get(id); t != nil {
		return t
	}
	return c.eth.Get(id)
}

// Remove tries to drop by hash from either sub-pool.  Returns true if removed.
func (c *TxPool) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool {
	if c.vechain.Remove(txHash, txID) {
		return true
	}
	return c.eth.Remove(txHash, txID)
}

// Len is the combined length across sub-pools.
func (c *TxPool) Len() int {
	return c.vechain.Len() + c.eth.Len()
}

// Dump concatenates snapshots from both sub-pools.
func (c *TxPool) Dump() tx.Transactions {
	v := c.vechain.Dump()
	e := c.eth.Dump()
	out := make(tx.Transactions, 0, len(v)+len(e))
	out = append(out, v...)
	out = append(out, e...)
	return out
}

// Fill routes each tx into the correct sub-pool.
func (c *TxPool) Fill(txs tx.Transactions) {
	var vecTxs, ethTxs tx.Transactions
	for _, t := range txs {
		if t.IsEthereumTx() {
			ethTxs = append(ethTxs, t)
		} else {
			vecTxs = append(vecTxs, t)
		}
	}
	if len(vecTxs) > 0 {
		c.vechain.Fill(vecTxs)
	}
	if len(ethTxs) > 0 {
		c.eth.Fill(ethTxs)
	}
}

// PoolNonce delegates to EthPool.
func (c *TxPool) PoolNonce(addr thor.Address) uint64 {
	return c.eth.PoolNonce(addr)
}

// GetByHash probes both sub-pools by RLP/wire hash and returns the first
// match. Collision is not a concern: the two families hash different
// inputs with different algorithms (VeChain-native uses Blake2b over RLP
// body; Ethereum-family uses Keccak256 over the EIP-2718 envelope), and
// hashes are cached per-transaction so the double-lookup is cheap.
//
// We probe VeChain first because it's the more common traffic in
// production; the check is a single map read in each sub-pool.
func (c *TxPool) GetByHash(hash thor.Bytes32) *tx.Transaction {
	if t := c.vechain.GetByHash(hash); t != nil {
		return t
	}
	return c.eth.GetByHash(hash)
}

// Executables performs the cross-family EFS merge.
//
// Algorithm (k-way heap merge):
//
//   - Seed the heap with one entry per source: the front of the VeChain queue
//     (already globally sorted by priorityGasPrice desc) and the head of each
//     Eth sender's pending chain (ascending nonce).
//   - Pop highest-priority entry; emit it; advance its source.  For Eth, the
//     source cursor is the per-sender pending slice (so the next nonce from
//     that sender is considered next — never the one after).  This guarantees
//     the packer never sees nonce N+1 before N for the same sender.
//
// The heap holds at most 1 + numEthSenders entries, so the merge is O(N log S)
// where N is the total tx count and S is the number of Eth senders.
func (c *TxPool) Executables() tx.Transactions {
	vObjs := c.vechain.executablesSorted()
	ethGroups := c.eth.executablePendingGroups()

	total := len(vObjs)
	for _, g := range ethGroups {
		total += len(g)
	}
	if total == 0 {
		return nil
	}

	h := &mergeHeap{}
	heap.Init(h)

	if len(vObjs) > 0 {
		heap.Push(h, mergeEntry{
			priority: priorityOf(vObjs[0]),
			source:   sourceVeChain,
			idx:      0,
			txObj:    vObjs[0],
		})
	}
	for gi, g := range ethGroups {
		heap.Push(h, mergeEntry{
			priority: priorityOf(g[0]),
			source:   sourceEth,
			idx:      0,
			group:    gi,
			txObj:    g[0],
		})
	}

	out := make(tx.Transactions, 0, total)
	for h.Len() > 0 {
		top := heap.Pop(h).(mergeEntry)
		out = append(out, top.txObj.Transaction)

		switch top.source {
		case sourceVeChain:
			next := top.idx + 1
			if next < len(vObjs) {
				heap.Push(h, mergeEntry{
					priority: priorityOf(vObjs[next]),
					source:   sourceVeChain,
					idx:      next,
					txObj:    vObjs[next],
				})
			}
		case sourceEth:
			next := top.idx + 1
			g := ethGroups[top.group]
			if next < len(g) {
				heap.Push(h, mergeEntry{
					priority: priorityOf(g[next]),
					source:   sourceEth,
					idx:      next,
					group:    top.group,
					txObj:    g[next],
				})
			}
		}
	}
	return out
}

// --- merge heap plumbing ---------------------------------------------------

type mergeSource uint8

const (
	sourceVeChain mergeSource = iota
	sourceEth
)

// mergeEntry is a heap element.  For Eth entries, group identifies the sender
// slice and idx is the cursor within that slice.  For VeChain entries, idx is
// the cursor within the single global VeChain slice.
type mergeEntry struct {
	priority *big.Int
	source   mergeSource
	idx      int
	group    int // eth sender group index; unused for VeChain
	txObj    *TxObject
}

type mergeHeap []mergeEntry

func (h mergeHeap) Len() int { return len(h) }
func (h mergeHeap) Less(i, j int) bool {
	// descending priority
	return h[i].priority.Cmp(h[j].priority) > 0
}
func (h mergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *mergeHeap) Push(x any) { *h = append(*h, x.(mergeEntry)) }
func (h *mergeHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// priorityOf is a nil-safe accessor for TxObject.priorityGasPrice.  A zero
// priority keeps non-executable / freshly-added txs from corrupting the heap
// comparator during early-life windows when Executable hasn't filled the field.
func priorityOf(o *TxObject) *big.Int {
	if o == nil || o.priorityGasPrice == nil {
		return big.NewInt(0)
	}
	return o.priorityGasPrice
}

// compile-time assertion that TxPool implements Pool.
var _ Pool = (*TxPool)(nil)
