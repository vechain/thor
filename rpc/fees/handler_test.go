// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees_test

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc/fees"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
)

type fixture struct {
	chain *testchain.Chain
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	require.NoError(t, c.MintBlock())
	return &fixture{chain: c}
}

func TestFeesHandler(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, fees.New(fx.chain.Repo(), 100, &testchain.DefaultForkConfig))

	t.Run("eth_gasPrice", func(t *testing.T) {
		// gasPrice = baseFee + 1 gwei tip; must be > 0 after GALACTICA.
		result := testutil.Call(t, ts, "eth_gasPrice", []any{})
		var price hexutil.Big
		require.NoError(t, json.Unmarshal(result, &price))
		assert.True(t, price.ToInt().Sign() > 0)
	})

	t.Run("eth_maxPriorityFeePerGas", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_maxPriorityFeePerGas", []any{})
		var tip hexutil.Big
		require.NoError(t, json.Unmarshal(result, &tip))
		assert.True(t, tip.ToInt().Sign() > 0)
	})

	t.Run("eth_feeHistory_single_block", func(t *testing.T) {
		// blockCount=1, newestBlock="latest"
		result := testutil.Call(t, ts, "eth_feeHistory", []any{1, "latest", []any{}})
		var fh map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &fh))

		// baseFeePerGas has length blockCount+1 = 2 (includes next-block estimate).
		var baseFees []*hexutil.Big
		require.NoError(t, json.Unmarshal(fh["baseFeePerGas"], &baseFees))
		assert.Len(t, baseFees, 2)

		// gasUsedRatio has length blockCount = 1.
		var gasRatios []float64
		require.NoError(t, json.Unmarshal(fh["gasUsedRatio"], &gasRatios))
		assert.Len(t, gasRatios, 1)

		// oldestBlock is the first block in the range.
		var oldest hexutil.Uint64
		require.NoError(t, json.Unmarshal(fh["oldestBlock"], &oldest))
		assert.Equal(t, uint64(1), uint64(oldest))
	})

	t.Run("eth_feeHistory_zero_blockCount", func(t *testing.T) {
		rpcErr := testutil.CallExpectError(t, ts, "eth_feeHistory", []any{0, "latest", []any{}})
		assert.NotNil(t, rpcErr)
	})

	t.Run("eth_feeHistory_reward_percentiles_unsupported", func(t *testing.T) {
		// Non-empty rewardPercentiles must return a server error rather than silently
		// returning an empty reward array.
		rpcErr := testutil.CallExpectError(t, ts, "eth_feeHistory", []any{1, "latest", []float64{25, 50, 75}})
		assert.NotNil(t, rpcErr)
		assert.Contains(t, rpcErr.Message, "reward percentiles are not yet supported")
	})
}

// TestFeeHistoryGasUsedRatioEthOnly verifies that gasUsedRatio counts only
// TypeEthDynamicFee gas, not VeChain legacy tx gas.
func TestFeeHistoryGasUsedRatioEthOnly(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	// Mint a block containing only a VeChain legacy tx.
	// It consumes gas (GasUsed > 0 at the block level) but is not ETH-typed.
	vcTx := testutil.BuildVcTx(t, c, sender, &recipient.Address)
	require.NoError(t, c.MintBlock(vcTx))

	ts := testutil.NewTestServer(t, fees.New(c.Repo(), 100, &testchain.DefaultForkConfig))

	result := testutil.Call(t, ts, "eth_feeHistory", []any{1, "latest", []any{}})
	var fh map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &fh))

	var gasRatios []float64
	require.NoError(t, json.Unmarshal(fh["gasUsedRatio"], &gasRatios))
	require.Len(t, gasRatios, 1)
	assert.Equal(t, 0.0, gasRatios[0],
		"VeChain legacy tx gas must not contribute to gasUsedRatio")
}
