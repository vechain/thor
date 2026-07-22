// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"container/heap"
	"math/big"

	"github.com/vechain/thor/v2/tx"
)

// executableTx is an immutable view of the fields needed to order a pooled
// transaction. Snapshots retain the priority pointer that was current when
// they were built; repricing replaces that pointer rather than mutating it.
type executableTx struct {
	tx               *tx.Transaction
	priorityGasPrice *big.Int
	timeAdded        int64
}

func executableTxFromObject(txObj *TxObject) executableTx {
	return executableTx{
		tx:               txObj.Transaction,
		priorityGasPrice: txObj.priorityGasPrice,
		timeAdded:        txObj.timeAdded,
	}
}

type vechainExecutablesSnapshot struct {
	transactions tx.Transactions
	entries      []executableTx
}

type ethExecutablesSnapshot struct {
	groups [][]executableTx
	total  int
}

func (s ethExecutablesSnapshot) transactions() tx.Transactions {
	return orderExecutableStreams(s.groups, s.total)
}

// compareExecutableTx returns a negative value when a sorts before b.
func compareExecutableTx(a, b executableTx) int {
	if cmp := b.priorityGasPrice.Cmp(a.priorityGasPrice); cmp != 0 {
		return cmp
	}
	switch {
	case a.timeAdded > b.timeAdded:
		return -1
	case a.timeAdded < b.timeAdded:
		return 1
	default:
		return 0
	}
}

type executableMergeItem struct {
	entry       executableTx
	sourceIndex int
	entryIndex  int
}

type executableMergeHeap []executableMergeItem

func (h executableMergeHeap) Len() int {
	return len(h)
}

func (h executableMergeHeap) Less(i, j int) bool {
	if cmp := compareExecutableTx(h[i].entry, h[j].entry); cmp != 0 {
		return cmp < 0
	}
	// Exact priority/time ties have no existing ordering contract. Source order
	// gives heap.Interface a strict, stable ordering without hashing txs.
	return h[i].sourceIndex < h[j].sourceIndex
}

func (h executableMergeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *executableMergeHeap) Push(value any) {
	*h = append(*h, value.(executableMergeItem))
}

func (h *executableMergeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = executableMergeItem{}
	*h = old[:n-1]
	return item
}

func mergePoolExecutables(vechain []executableTx, eth ethExecutablesSnapshot) tx.Transactions {
	if len(vechain) == 0 && eth.total == 0 {
		return nil
	}

	sources := make([][]executableTx, 0, 1+len(eth.groups))
	if len(vechain) > 0 {
		sources = append(sources, vechain)
	}
	sources = append(sources, eth.groups...)

	return orderExecutableStreams(sources, len(vechain)+eth.total)
}

func orderExecutableStreams(sources [][]executableTx, total int) tx.Transactions {
	if total == 0 {
		return nil
	}

	mergeHeap := make(executableMergeHeap, 0, len(sources))
	for sourceIndex, source := range sources {
		if len(source) == 0 {
			continue
		}
		mergeHeap = append(mergeHeap, executableMergeItem{
			entry:       source[0],
			sourceIndex: sourceIndex,
		})
	}
	heap.Init(&mergeHeap)

	ordered := make(tx.Transactions, 0, total)
	for mergeHeap.Len() > 0 {
		item := heap.Pop(&mergeHeap).(executableMergeItem)
		ordered = append(ordered, item.entry.tx)

		item.entryIndex++
		source := sources[item.sourceIndex]
		if item.entryIndex < len(source) {
			item.entry = source[item.entryIndex]
			heap.Push(&mergeHeap, item)
		}
	}
	return ordered
}
