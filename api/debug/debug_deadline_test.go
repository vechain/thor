// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package debug

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// gasBurnerInitCode is CREATE init code that loops until its gas runs out:
// JUMPDEST, PUSH1 0, JUMP.
const gasBurnerInitCode = "0x5b600056"

func newDeadlineTestChain(t *testing.T) *testchain.Chain {
	t.Helper()
	forkConfig := thor.ForkConfig{
		BLOCKLIST:    0,
		VIP191:       1,
		GALACTICA:    1,
		VIP214:       2,
		HAYABUSA:     math.MaxUint32,
		INTERSTELLAR: math.MaxUint32,
	}
	hayabusaTP := uint32(math.MaxUint32)
	thor.SetConfig(thor.Config{HayabusaTP: &hayabusaTP})
	thorChain, err := testchain.NewWithFork(&forkConfig, 180)
	require.NoError(t, err)
	return thorChain
}

// TestReplayStopsWithContext pins that replaying the clauses before the traced one
// stops when ctx does: the block's first transaction burns most of the block's gas,
// and a trace of the second must give up at the deadline instead of finishing the
// replay first.
func TestReplayStopsWithContext(t *testing.T) {
	thorChain := newDeadlineTestChain(t)

	burner := tx.MustSign(tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(burnerGas).
		Nonce(1).
		Clause(tx.NewClause(nil).WithData(hexutil.MustDecode(gasBurnerInitCode))).
		BlockRef(tx.NewBlockRef(0)).
		Build(), genesis.DevAccounts()[0].PrivateKey)
	to := thor.BytesToAddress([]byte("to"))
	target := tx.MustSign(tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(21000).
		Nonce(2).
		Clause(tx.NewClause(&to)).
		BlockRef(tx.NewBlockRef(0)).
		Build(), genesis.DevAccounts()[1].PrivateKey)
	require.NoError(t, thorChain.MintBlock(burner, target))

	d := New(thorChain.Repo(), thorChain.Stater(), thorChain.GetForkConfig(), thorChain.Engine(),
		50_000_000, true, []string{"all"}, false, testTraceDataMaxSize)
	block, txID, clauseIndex, err := d.parseTarget(target.ID().String() + "/0")
	require.NoError(t, err)

	// The full replay is the yardstick: a deadline at a small fraction of it must
	// cut the replay short. A first replay warms the caches, so the yardstick is
	// not inflated against the timed runs.
	_, _, _, err = d.prepareClauseEnv(context.Background(), block, txID, clauseIndex)
	require.NoError(t, err)
	start := time.Now()
	_, _, _, err = d.prepareClauseEnv(context.Background(), block, txID, clauseIndex)
	require.NoError(t, err)
	full := time.Since(start)

	ctx, cancel := context.WithTimeout(context.Background(), full/10)
	defer cancel()
	start = time.Now()
	_, _, _, err = d.prepareClauseEnv(ctx, block, txID, clauseIndex)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, full/2, "replay ran %s of a %s full replay past a %s deadline", elapsed, full, full/10)
}

// TestCustomTracerStopsWithContext pins that caller-supplied JS cannot outlive the
// request: neither a step function that never returns (stopped while the EVM
// runs) nor a result function that never returns (stopped during GetResult).
func TestCustomTracerStopsWithContext(t *testing.T) {
	thorChain := newDeadlineTestChain(t)
	d := New(thorChain.Repo(), thorChain.Stater(), thorChain.GetForkConfig(), thorChain.Engine(),
		500_000, true, []string{"all"}, false, testTraceDataMaxSize)

	for name, code := range map[string]string{
		"step never returns":   `{step: function() { while (true) {} }, fault: function() {}, result: function() { return 1 }}`,
		"result never returns": `{step: function() {}, fault: function() {}, result: function() { while (true) {} }}`,
	} {
		t.Run(name, func(t *testing.T) {
			tracer, native, err := d.createTracer(code, nil)
			require.NoError(t, err)
			require.False(t, native)

			rev, err := restutil.ParseRevision("best", true)
			require.NoError(t, err)
			summary, st, err := restutil.GetSummaryAndState(rev,
				thorChain.Repo(), thorChain.Engine(), thorChain.Stater(), thorChain.GetForkConfig())
			require.NoError(t, err)
			// PUSH1 0, POP, STOP: a few opcodes, so step runs.
			txCtx, gas, clause, err := d.handleTraceCallOption(&api.TraceCallOption{Data: "0x60005000", Gas: 50000})
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			type result struct {
				res any
				err error
			}
			done := make(chan result, 1)
			go func() {
				res, err := d.traceCall(ctx, tracer, native, summary.Header, st, txCtx, gas, clause)
				done <- result{res, err}
			}()

			select {
			case r := <-done:
				assert.Nil(t, r.res)
				assert.ErrorIs(t, r.err, context.DeadlineExceeded)
			case <-time.After(5 * time.Second):
				t.Fatal("custom tracer outlived the request deadline")
			}
		})
	}
}
