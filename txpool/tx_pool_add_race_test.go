// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// TestConcurrentAddSameTxDoesNotDoubleCountMetric reproduces the scenario a
// well-connected node sees constantly in production: the same transaction
// gossiped in from several peers at nearly the same moment, so multiple
// goroutines call Add with an identical hash concurrently.
//
// Before the fix, TxPool.add's unlocked ContainsHash pre-check could let more
// than one goroutine reach p.all.Add for the same hash; only one would
// actually insert (the lock inside Add correctly dedups), but every caller
// that observed a nil error - including the no-op "already exists" case -
// unconditionally incremented the thor_metrics_txpool_current_tx_count gauge.
// Only one real object ever exists to be removed later, so every extra
// increment was a permanent leak with no corresponding future decrement.
//
// Run with -race to also confirm no data race is introduced by the fix.
func TestConcurrentAddSameTxDoesNotDoubleCountMetric(t *testing.T) {
	metrics.InitializePrometheusMetrics()

	// Generous limits so Add is never rejected on quota/pool-full - every
	// goroutine must reach the real dedup point (p.all.Add) to exercise it.
	pool := newPool(4000, 4000, &thor.NoFork)
	defer pool.Close()

	trx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])

	// thor_metrics_txpool_current_tx_count is a process-wide Prometheus
	// singleton, not scoped to this pool - measure the delta this run
	// produces rather than an absolute value, so the assertion holds even
	// when this test itself runs more than once in the same process
	// (e.g. `go test -count=2`).
	before := sumGaugeValues(t, "thor_metrics_txpool_current_tx_count")

	const numRacers = 32
	var wg sync.WaitGroup
	ready := make(chan struct{})
	for range numRacers {
		wg.Go(func() {
			<-ready // maximize the race window: all goroutines fire together
			_ = pool.Add(trx)
		})
	}
	close(ready)
	wg.Wait()

	assert.Equal(t, 1, pool.Len(), "only one copy of the tx should actually be in the pool")

	after := sumGaugeValues(t, "thor_metrics_txpool_current_tx_count")
	assert.Equal(t, float64(1), after-before, "gauge must count the single real insertion exactly once, not once per racing caller")

	// thor_metrics_txpool_current_tx_count is a process-wide singleton (see
	// above) - remove the one real tx we added so this test doesn't leave a
	// permanent +1 behind for other tests in this package that read the same
	// gauge with an absolute (not delta) assertion.
	pool.Remove(trx.Hash(), trx.ID())
}

// sumGaugeValues sums every label-combination value of a gauge metric family.
func sumGaugeValues(t *testing.T, name string) float64 {
	t.Helper()

	mf := gatherMetricFamily(t, name)
	if mf == nil {
		return 0
	}
	var total float64
	for _, m := range mf.GetMetric() {
		total += m.GetGauge().GetValue()
	}
	return total
}
