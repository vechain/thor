// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"container/heap"
	"math/big"
	"testing"

	"github.com/vechain/thor/v2/tx"
)

// synthMergeTxObj builds a minimal TxObject with the given priority.
// Only the fields accessed by the merge algorithm are populated.
func synthMergeTxObj(priority int64) *TxObject {
	trx := tx.NewBuilder(tx.TypeLegacy).Gas(21000).Build()
	return &TxObject{
		Transaction:      trx,
		priorityGasPrice: big.NewInt(priority),
	}
}

// benchExecutablesMerge exercises the k-way merge with the given configuration.
// nVeChain VeChain txs (single sorted slice), nEthSenders Eth senders each
// with nNoncesPerSender pending txs.
func benchExecutablesMerge(b *testing.B, nVeChain, nEthSenders, nNoncesPerSender int) {
	b.Helper()

	// Pre-build the input slices once; the merge is read-only on them.
	vObjs := make([]*TxObject, nVeChain)
	for i := range vObjs {
		vObjs[i] = synthMergeTxObj(int64(nVeChain - i)) // descending priority
	}

	ethGroups := make([][]*TxObject, nEthSenders)
	for s := range ethGroups {
		g := make([]*TxObject, nNoncesPerSender)
		for n := range g {
			// Head of each sender has the highest priority; subsequent nonces
			// decrease. Different senders get slightly different base priorities.
			g[n] = synthMergeTxObj(int64(nEthSenders-s)*100 - int64(n))
		}
		ethGroups[s] = g
	}

	total := nVeChain + nEthSenders*nNoncesPerSender

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
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

		if len(out) != total {
			b.Fatalf("expected %d merged txs, got %d", total, len(out))
		}
	}
}

// --- Pure VeChain (single source, degenerate heap) -------------------------

func BenchmarkMerge_VeChainOnly_100(b *testing.B) {
	benchExecutablesMerge(b, 100, 0, 0)
}

func BenchmarkMerge_VeChainOnly_1000(b *testing.B) {
	benchExecutablesMerge(b, 1000, 0, 0)
}

func BenchmarkMerge_VeChainOnly_5000(b *testing.B) {
	benchExecutablesMerge(b, 5000, 0, 0)
}

// --- Pure Eth (many senders, nonce-ordered per sender) ---------------------

func BenchmarkMerge_EthOnly_10senders_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 0, 10, 10)
}

func BenchmarkMerge_EthOnly_100senders_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 0, 100, 10)
}

func BenchmarkMerge_EthOnly_500senders_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 0, 500, 10)
}

func BenchmarkMerge_EthOnly_100senders_64nonces(b *testing.B) {
	benchExecutablesMerge(b, 0, 100, 64)
}

// --- Mixed families --------------------------------------------------------

func BenchmarkMerge_Mixed_500vc_50eth_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 500, 50, 10)
}

func BenchmarkMerge_Mixed_1000vc_100eth_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 1000, 100, 10)
}

func BenchmarkMerge_Mixed_2000vc_200eth_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 2000, 200, 10)
}

func BenchmarkMerge_Mixed_5000vc_500eth_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 5000, 500, 10)
}

// --- Stress: large heap with many senders ----------------------------------

func BenchmarkMerge_Stress_0vc_1000eth_64nonces(b *testing.B) {
	benchExecutablesMerge(b, 0, 1000, 64)
}

func BenchmarkMerge_Stress_5000vc_1000eth_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 5000, 1000, 10)
}
