// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package simulation_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/simulation"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

type fixture struct {
	chain         *testchain.Chain
	forks         thor.ForkConfig
	senderAddr    string
	recipientAddr string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	// No block minted — genesis dev accounts are funded and simulation runs
	// against the latest state directly.
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]
	return &fixture{
		chain:         c,
		forks:         testchain.DefaultForkConfig,
		senderAddr:    sender.Address.String(),
		recipientAddr: recipient.Address.String(),
	}
}

func TestSimulationHandler(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, simulation.New(
		fx.chain.Repo(), fx.chain.Stater(), &fx.forks, 1_000_000,
	))

	t.Run("eth_call_transfer", func(t *testing.T) {
		// A plain VET transfer returns empty output data.
		result := testutil.Call(t, ts, "eth_call", []any{
			map[string]any{
				"from":  fx.senderAddr,
				"to":    fx.recipientAddr,
				"value": "0x1",
			},
			"latest",
		})
		var data hexutil.Bytes
		require.NoError(t, json.Unmarshal(result, &data))
		assert.Empty(t, data)
	})

	t.Run("eth_estimateGas_transfer", func(t *testing.T) {
		// A simple EOA-to-EOA transfer costs exactly 21000 gas:
		// 5000 (tx base) + 16000 (per-clause) = 21000 intrinsic, 0 EVM gas.
		result := testutil.Call(t, ts, "eth_estimateGas", []any{
			map[string]any{
				"from":  fx.senderAddr,
				"to":    fx.recipientAddr,
				"value": "0x1",
			},
		})
		var gasEst hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &gasEst))
		assert.Equal(t, uint64(21000), uint64(gasEst))
	})

	t.Run("eth_estimateGas_respects_gas_cap", func(t *testing.T) {
		// Providing a gas cap lower than the intrinsic cost should still succeed
		// for a zero-opcode call (EVM gas used = 0, only intrinsic matters).
		// Here we pass gas = 21000 which is exactly the estimate.
		result := testutil.Call(t, ts, "eth_estimateGas", []any{
			map[string]any{
				"from":  fx.senderAddr,
				"to":    fx.recipientAddr,
				"value": "0x1",
				"gas":   "0x5208", // 0x5208 = 21000
			},
		})
		var gasEst hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &gasEst))
		assert.Equal(t, uint64(21000), uint64(gasEst))
	})

	t.Run("eth_call_revert", func(t *testing.T) {
		// Call Energy.transfer from a zero-VTHO address — the balance check reverts.
		transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
		require.True(t, ok)
		callData, err := transferMethod.EncodeInput(
			genesis.DevAccounts()[1].Address, big.NewInt(1),
		)
		require.NoError(t, err)

		rpcErr := testutil.CallExpectError(t, ts, "eth_call", []any{
			map[string]any{
				"from": "0x000000000000000000000000000000000000000a", // no VTHO
				"to":   builtin.Energy.Address.String(),
				"data": hexutil.Encode(callData),
			},
			"latest",
		})
		assert.Equal(t, rpc.CodeServerError, rpcErr.Code)
		assert.Equal(t, "execution reverted", rpcErr.Message)
	})

	t.Run("eth_estimateGas_revert", func(t *testing.T) {
		// Same call via eth_estimateGas — must also return a server error.
		transferMethod, ok := builtin.Energy.ABI.MethodByName("transfer")
		require.True(t, ok)
		callData, err := transferMethod.EncodeInput(
			genesis.DevAccounts()[1].Address, big.NewInt(1),
		)
		require.NoError(t, err)

		rpcErr := testutil.CallExpectError(t, ts, "eth_estimateGas", []any{
			map[string]any{
				"from": "0x000000000000000000000000000000000000000a",
				"to":   builtin.Energy.Address.String(),
				"data": hexutil.Encode(callData),
			},
		})
		assert.Equal(t, rpc.CodeServerError, rpcErr.Code)
	})
}
