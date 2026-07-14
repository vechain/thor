// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// TestConcurrentWashAddRemove drives wash concurrently with Add/Remove over a
// churning pool; run with -race. It targets the historical data race between
// wash publishing pricing/promoting txs and Add/Remove reading pricing and
// mutating the pending-cost accounting (pool.all.cost).
//
// Assertions: no panic, no data race (detected by the race detector, not by
// this test's logic), and the pending-cost map is fully drained once every
// tx added by the workers has been removed and wash has stopped running.
func TestConcurrentWashAddRemove(t *testing.T) {
	// Generous limits so Add is never rejected purely on quota/pool-full;
	// txs must actually enter the pool and get washed/promoted to exercise
	// the accounting path.
	pool := newPool(4000, 4000, &thor.NoFork)
	defer pool.Close()

	best := pool.repo.BestBlockSummary()

	const numAddRemoveWorkers = 8
	// Exactly one wash goroutine: production runs a single serial housekeeping
	// washer (TxPool.housekeeping), and wash's lock-free reads of the plain
	// executable field rely on that single-writer model. Spinning multiple
	// washers would fabricate a race that cannot occur in production.
	const numWashWorkers = 1

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Add/Remove workers: each spins signing and adding a fresh tx (random
	// nonce guarantees a distinct hash), then immediately removes it. Every
	// tx it successfully adds is also removed by the same goroutine, so by
	// the time all workers join, every worker-added tx has had a matching
	// Remove call (whether or not wash got to it first).
	for w := range numAddRemoveWorkers {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				acc := devAccounts[(workerIdx+i)%len(devAccounts)]
				trx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)
				_ = pool.Add(trx)
				pool.Remove(trx.Hash(), trx.ID())
				i++
			}
		}(w)
	}

	// Wash workers: repeatedly evaluate/evict/promote against the churning
	// pool, exercising the lock-free pricing snapshot publish/read path.
	for range numWashWorkers {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _, _ = pool.wash(best, true)
			}
		})
	}

	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()

	pool.all.lock.RLock()
	remaining := len(pool.all.cost)
	pool.all.lock.RUnlock()
	assert.Zero(t, remaining, "pending cost map should be drained after all txs removed")
}
