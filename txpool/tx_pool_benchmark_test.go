// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// newBenchPool creates a pool with generous limits so that Add is never
// rejected due to quota/pool-full; the benchmark only measures the
// concurrent read/write overhead of Add/Remove(+wash).
func newBenchPool() *TxPool {
	return newPool(1_000_000, 1_000_000, &thor.NoFork)
}

// genBenchTxs pre-signs a batch of transactions with distinct hashes (newTx
// uses a random nonce, so even the same account yields distinct hashes).
// Signing is ECDSA and expensive, so it must happen outside the timed region.
func genBenchTxs(pool *TxPool, n, workerIdx int) []*tx.Transaction {
	txs := make([]*tx.Transaction, n)
	for i := 0; i < n; i++ {
		from := devAccounts[(workerIdx+i)%len(devAccounts)]
		txs[i] = newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), from)
	}
	return txs
}

func benchAddRemove(b *testing.B, workers, perWorker int, withWash bool) {
	pool := newBenchPool()
	defer pool.Close()

	sets := make([][]*tx.Transaction, workers)
	for w := 0; w < workers; w++ {
		sets[w] = genBenchTxs(pool, perWorker, w)
	}

	var (
		stopWash atomic.Bool
		washDone chan struct{}
	)
	if withWash {
		best := pool.repo.BestBlockSummary()
		washDone = make(chan struct{})
		go func() {
			defer close(washDone)
			for !stopWash.Load() {
				_, _, _, _ = pool.wash(best, true)
			}
		}()
	}

	b.ReportAllocs()
	b.ResetTimer()

	var wg sync.WaitGroup
	per := b.N / workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(set []*tx.Transaction) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				trx := set[i%len(set)]
				_ = pool.Add(trx)                 // publishes pricing snapshot (atomic Store) + bookkeeping (map lock)
				pool.Remove(trx.Hash(), trx.ID()) // reads Cost()/Payer() (atomic Load) + bookkeeping (map lock)
			}
		}(sets[w])
	}
	wg.Wait()

	b.StopTimer()
	if withWash {
		stopWash.Store(true)
		<-washDone
	}
}

// BenchmarkPoolConcurrentAddRemove exercises pure concurrent Add/Remove.
func BenchmarkPoolConcurrentAddRemove(b *testing.B) {
	benchAddRemove(b, 8, 256, false)
}

// BenchmarkPoolConcurrentAddRemoveWithWash exercises Add/Remove concurrently
// with a background wash: this directly stresses the atomic Store (wash
// publishing pricing) vs. atomic Load (Remove reading pricing) path.
func BenchmarkPoolConcurrentAddRemoveWithWash(b *testing.B) {
	benchAddRemove(b, 8, 256, true)
}
