// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package debug

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tracers"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vm"
)

// traceDataBudgetPayload is CREATE init code (no "to" in the request) that
// hands a 32 KiB memory region to the tracer as CALL arguments in a flat-fee
// loop until gas runs out: JUMPDEST, then CALL(gas=0, addr=0, value=0,
// inOff=0, inSize=0x8000, retOff=0, retSize=0), POP, jump back. Operands are
// pushed deepest-first, gas last.
const traceDataBudgetPayload = "0x5b600060006180006000600060006000f150600056"

// testTraceDataMaxSize trips traceDataBudgetPayload on its third CALL.
const testTraceDataMaxSize = 64 * 1024

func newTestCallTracer(t *testing.T) tracers.Tracer {
	tracer, err := tracers.DefaultDirectory.New("callTracer", nil, false)
	require.NoError(t, err)
	return tracer
}

// driveFrames feeds the tracer a top frame with the given nested call inputs.
func driveFrames(tracer tracers.Tracer, inputs ...[]byte) {
	from, to := common.Address{0x01}, common.Address{0x02}
	tracer.CaptureClauseStart(1_000_000)
	tracer.CaptureStart(nil, from, to, false, []byte{0xaa}, 0, nil)
	for _, in := range inputs {
		tracer.CaptureEnter(vm.CALL, from, to, in, 1000, nil)
		tracer.CaptureExit([]byte{0xbb}, 500, nil)
	}
	tracer.CaptureEnd([]byte{0xcc}, 0, nil)
	tracer.CaptureClauseEnd(0)
}

func TestDataBudgetTracerUnderBudgetUnchanged(t *testing.T) {
	inputs := [][]byte{{0x01}, make([]byte, 1024)}

	plain := newTestCallTracer(t)
	driveFrames(plain, inputs...)
	want, err := plain.GetResult()
	require.NoError(t, err)

	budget := newDataBudgetTracer(newTestCallTracer(t), 4096)
	driveFrames(budget, inputs...)
	require.NoError(t, budget.err)
	got, err := budget.GetResult()
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(got))
}

func TestDataBudgetTracerTrips(t *testing.T) {
	// 1 (top input) + 2*(1000 input + 1 output) = 2003 bytes fit exactly;
	// the third frame's input does not.
	const max = 1 + 2*(1000+1)
	budget := newDataBudgetTracer(newTestCallTracer(t), max)
	driveFrames(budget, make([]byte, 1000), make([]byte, 1000), make([]byte, 1))

	require.Error(t, budget.err)
	assert.Contains(t, budget.err.Error(), "trace data exceeds limit of 2003 bytes")
	assert.Zero(t, budget.left)
}

func TestTraceDataBudget(t *testing.T) {
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

	// Plant the payload on chain once, as a replay attacker would.
	planted := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		Expiration(10).
		Gas(200000).
		Nonce(1).
		Clause(tx.NewClause(nil).WithData(hexutil.MustDecode(traceDataBudgetPayload))).
		BlockRef(tx.NewBlockRef(0)).
		Build()
	planted = tx.MustSign(planted, genesis.DevAccounts()[0].PrivateKey)
	require.NoError(t, thorChain.MintBlock(planted))

	const callGasLimit = 500000
	d := New(thorChain.Repo(), thorChain.Stater(), thorChain.GetForkConfig(), thorChain.Engine(),
		callGasLimit, true, []string{"all"}, false, testTraceDataMaxSize)
	router := mux.NewRouter()
	d.Mount(router, "/debug")
	server := httptest.NewServer(router)
	defer server.Close()

	t.Run("trace call fails past the data budget", func(t *testing.T) {
		res, status := postJSON(t, server.URL+"/debug/tracers/call", map[string]any{
			"name": "call",
			"data": traceDataBudgetPayload,
			"gas":  200000,
		})
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, res, "trace data exceeds limit of 65536 bytes")
	})

	t.Run("replay fails past the data budget", func(t *testing.T) {
		res, status := postJSON(t, server.URL+"/debug/tracers", map[string]any{
			"name":   "call",
			"target": planted.ID().String() + "/0",
		})
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, res, "trace data exceeds limit of 65536 bytes")
	})

	t.Run("trace call under the data budget succeeds", func(t *testing.T) {
		_, status := postJSON(t, server.URL+"/debug/tracers/call", map[string]any{
			"name": "call",
			"to":   thor.BytesToAddress([]byte("to")).String(),
			"gas":  50000,
		})
		assert.Equal(t, http.StatusOK, status)
	})

	t.Run("fails closed when the context is done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		tracer, native, err := d.createTracer("call", nil)
		require.NoError(t, err)
		rev, err := restutil.ParseRevision("best", true)
		require.NoError(t, err)
		summary, st, err := restutil.GetSummaryAndState(rev,
			thorChain.Repo(), thorChain.Engine(), thorChain.Stater(), thorChain.GetForkConfig())
		require.NoError(t, err)
		txCtx, gas, clause, err := d.handleTraceCallOption(&api.TraceCallOption{
			Name: "call",
			To:   new(thor.BytesToAddress([]byte("to"))),
			Gas:  50000,
		})
		require.NoError(t, err)

		// Even if execution completes, a fired deadline must prevent the
		// GetResult/WriteJSON tail from running.
		res, err := d.traceCall(ctx, tracer, native, summary.Header, st, txCtx, gas, clause)
		assert.Nil(t, res)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func postJSON(t *testing.T, url string, payload any) (string, int) {
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return string(resBody), res.StatusCode
}
