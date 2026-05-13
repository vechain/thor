// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"container/heap"
	"math/big"
	"testing"

	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/thor"
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

func synthAddress(i int) thor.Address {
	var addr thor.Address
	b := addr[:]
	v := uint64(i + 1)
	for j := len(b) - 1; j >= len(b)-8; j-- {
		b[j] = byte(v)
		v >>= 8
	}
	return addr
}

func synthPoolTxObj(origin thor.Address, nonce uint64, priority int64) *TxObject {
	trx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(0x27).
		Clause(tx.NewClause(&origin)).
		Gas(21000).
		Nonce(nonce).
		Build()
	return &TxObject{
		Transaction: trx,
		resolved: &runtime.ResolvedTransaction{
			Origin: origin,
		},
		payer:            &origin,
		cost:             big.NewInt(21000),
		priorityGasPrice: big.NewInt(priority),
		executable:       true,
	}
}

func synthEthPoolTxObj(sender, nonce int) *TxObject {
	origin := synthAddress(sender)
	return synthPoolTxObj(origin, uint64(nonce), int64(sender+nonce+1))
}

func synthVeChainPoolTxObj(origin, nonce int) *TxObject {
	return synthPoolTxObj(synthAddress(origin), uint64(nonce), int64(origin+nonce+1))
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
	heapHeads := nEthSenders
	if nVeChain > 0 {
		heapHeads++
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.ReportMetric(float64(total), "txs/op")
	b.ReportMetric(float64(heapHeads), "heap_heads/op")

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

// benchMergeHeapPushWorstCase measures fixed-size heap churn where each push
// inserts a new highest-priority entry, forcing the value to bubble to the root.
// The matching pop keeps heap size stable between iterations.
func benchMergeHeapPushWorstCase(b *testing.B, heapSize int) {
	b.Helper()

	h := make(mergeHeap, heapSize, heapSize+1)
	for i := range h {
		h[i] = mergeEntry{
			priority: big.NewInt(0),
			source:   sourceEth,
			idx:      0,
			group:    i,
			txObj:    synthMergeTxObj(0),
		}
	}
	heap.Init(&h)

	worst := mergeEntry{
		priority: big.NewInt(1),
		source:   sourceEth,
		idx:      0,
		txObj:    synthMergeTxObj(1),
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.ReportMetric(float64(heapSize), "heap_entries/op")

	for range b.N {
		heap.Push(&h, worst)
		heap.Pop(&h)
	}
}

func benchEthPoolMapAdmissionManySenders(b *testing.B, senders, noncesPerSender int) {
	b.Helper()

	total := senders * noncesPerSender
	txObjs := make([]*TxObject, 0, total)
	for s := range senders {
		for n := range noncesPerSender {
			txObjs = append(txObjs, synthEthPoolTxObj(s, n))
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.ReportMetric(float64(total), "txs/op")
	b.ReportMetric(float64(senders), "senders/op")
	b.ReportMetric(float64(total), "limit/op")

	for range b.N {
		m := newEthPoolMapWithLimit(total, noncesPerSender)
		for _, txObj := range txObjs {
			if _, err := m.add(txObj, 0); err != nil {
				b.Fatal(err)
			}
		}
		if got := m.len(); got != total {
			b.Fatalf("expected %d txs, got %d", total, got)
		}
	}
}

func benchEthPoolMapAdmissionOneSenderQuota(b *testing.B, accountLimit int) {
	b.Helper()

	total := accountLimit
	txObjs := make([]*TxObject, 0, total)
	pendingCount := accountLimit / 2
	for n := range pendingCount {
		txObjs = append(txObjs, synthEthPoolTxObj(0, n))
	}
	for n := range accountLimit - pendingCount {
		txObjs = append(txObjs, synthEthPoolTxObj(0, pendingCount+1+n))
	}
	overflowPending := synthEthPoolTxObj(0, pendingCount)
	overflowQueue := synthEthPoolTxObj(0, accountLimit+1)

	b.ResetTimer()
	b.ReportAllocs()
	b.ReportMetric(1, "senders/op")
	b.ReportMetric(float64(accountLimit), "account_limit/op")

	for range b.N {
		m := newEthPoolMapWithLimit(total+2, accountLimit)
		for _, txObj := range txObjs {
			if _, err := m.add(txObj, 0); err != nil {
				b.Fatal(err)
			}
		}
		if _, err := m.add(overflowPending, 0); err == nil {
			b.Fatal("expected account quota error")
		}
		if _, err := m.add(overflowQueue, 0); err == nil {
			b.Fatal("expected account quota error")
		}
		if got := m.len(); got != total {
			b.Fatalf("expected %d txs, got %d", total, got)
		}
	}
}

func benchTxObjectMapAdmissionManyOrigins(b *testing.B, origins, txsPerOrigin int) {
	b.Helper()

	total := origins * txsPerOrigin
	txObjs := make([]*TxObject, 0, total)
	for origin := range origins {
		for n := range txsPerOrigin {
			txObjs = append(txObjs, synthVeChainPoolTxObj(origin, origin*txsPerOrigin+n))
		}
	}
	validatePayer := func(_ thor.Address, _ *big.Int) error { return nil }

	b.ResetTimer()
	b.ReportAllocs()
	b.ReportMetric(float64(total), "txs/op")
	b.ReportMetric(float64(origins), "origins/op")
	b.ReportMetric(float64(txsPerOrigin), "quota/op")

	for range b.N {
		m := newTxObjectMap()
		for _, txObj := range txObjs {
			if err := m.Add(txObj, txsPerOrigin, validatePayer); err != nil {
				b.Fatal(err)
			}
		}
		if got := m.Len(); got != total {
			b.Fatalf("expected %d txs, got %d", total, got)
		}
	}
}

func benchEthExecutablesRepeated(b *testing.B, senders, noncesPerSender int) {
	b.Helper()

	total := senders * noncesPerSender
	m := newEthPoolMapWithLimit(total, noncesPerSender)
	for s := range senders {
		for n := range noncesPerSender {
			if _, err := m.add(synthEthPoolTxObj(s, n), 0); err != nil {
				b.Fatal(err)
			}
		}
	}
	p := &EthPool{pool: m}

	b.ResetTimer()
	b.ReportAllocs()
	b.ReportMetric(float64(total), "txs/op")
	b.ReportMetric(float64(senders), "senders/op")

	for range b.N {
		execs := p.Executables()
		if len(execs) != total {
			b.Fatalf("expected %d executables, got %d", total, len(execs))
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

func BenchmarkMerge_VeChainOnly_12000(b *testing.B) {
	benchExecutablesMerge(b, 12000, 0, 0)
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

func BenchmarkMerge_EthOnly_1200senders_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 0, 1200, 10)
}

func BenchmarkMerge_EthOnly_6000senders_2nonces(b *testing.B) {
	benchExecutablesMerge(b, 0, 6000, 2)
}

func BenchmarkMerge_EthOnly_12000senders_1nonce(b *testing.B) {
	benchExecutablesMerge(b, 0, 12000, 1)
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

func BenchmarkMerge_Mixed_12000vc_1200eth_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 12000, 1200, 10)
}

func BenchmarkMerge_Mixed_12000vc_12000eth_1nonce(b *testing.B) {
	benchExecutablesMerge(b, 12000, 12000, 1)
}

// --- Stress: large heap with many senders ----------------------------------

func BenchmarkMerge_Stress_0vc_1000eth_64nonces(b *testing.B) {
	benchExecutablesMerge(b, 0, 1000, 64)
}

func BenchmarkMerge_Stress_5000vc_1000eth_10nonces(b *testing.B) {
	benchExecutablesMerge(b, 5000, 1000, 10)
}

// --- Heap push stress -------------------------------------------------------

func BenchmarkMergeHeapPushWorstCase_600entries(b *testing.B) {
	benchMergeHeapPushWorstCase(b, 600)
}

func BenchmarkMergeHeapPushWorstCase_1200entries(b *testing.B) {
	benchMergeHeapPushWorstCase(b, 1200)
}

func BenchmarkMergeHeapPushWorstCase_5000entries(b *testing.B) {
	benchMergeHeapPushWorstCase(b, 5000)
}

func BenchmarkMergeHeapPushWorstCase_12000entries(b *testing.B) {
	benchMergeHeapPushWorstCase(b, 12000)
}

// --- Admission and metadata growth -----------------------------------------

func BenchmarkEthPoolMapAdmission_ManySenders_1200x10(b *testing.B) {
	benchEthPoolMapAdmissionManySenders(b, 1200, 10)
}

func BenchmarkEthPoolMapAdmission_ManySenders_12000x1(b *testing.B) {
	benchEthPoolMapAdmissionManySenders(b, 12000, 1)
}

func BenchmarkEthPoolMapAdmission_OneSender_128Quota(b *testing.B) {
	benchEthPoolMapAdmissionOneSenderQuota(b, 128)
}

func BenchmarkTxObjectMapAdmission_ManyOrigins_1200x10(b *testing.B) {
	benchTxObjectMapAdmissionManyOrigins(b, 1200, 10)
}

func BenchmarkTxObjectMapAdmission_ManyOrigins_12000x1(b *testing.B) {
	benchTxObjectMapAdmissionManyOrigins(b, 12000, 1)
}

// --- Repeated executable snapshots -----------------------------------------

func BenchmarkExecutablesRepeated_EthOnly_1200senders_10nonces(b *testing.B) {
	benchEthExecutablesRepeated(b, 1200, 10)
}

func BenchmarkExecutablesRepeated_EthOnly_12000senders_1nonce(b *testing.B) {
	benchEthExecutablesRepeated(b, 12000, 1)
}
