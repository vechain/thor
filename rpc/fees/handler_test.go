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

	t.Run("eth_feeHistory_reward_percentiles", func(t *testing.T) {
		// Block 1 from the fixture is empty, so reward is present with one entry
		// per block and one value per requested percentile, all zero.
		result := testutil.Call(t, ts, "eth_feeHistory", []any{1, "latest", []float64{25, 50, 75}})
		var fh map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &fh))

		var rewards [][]*hexutil.Big
		require.NoError(t, json.Unmarshal(fh["reward"], &rewards))
		require.Len(t, rewards, 1)
		require.Len(t, rewards[0], 3)
		for _, r := range rewards[0] {
			assert.Equal(t, 0, r.ToInt().Sign())
		}
	})

	t.Run("eth_feeHistory_no_reward_when_percentiles_empty", func(t *testing.T) {
		// Omitting rewardPercentiles must omit the reward field entirely.
		result := testutil.Call(t, ts, "eth_feeHistory", []any{1, "latest", []any{}})
		var fh map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &fh))
		_, present := fh["reward"]
		assert.False(t, present)
	})

	t.Run("eth_feeHistory_invalid_reward_percentiles", func(t *testing.T) {
		// Descending percentiles are invalid.
		rpcErr := testutil.CallExpectError(t, ts, "eth_feeHistory", []any{1, "latest", []float64{75, 25}})
		assert.NotNil(t, rpcErr)
	})
}

// TestFeeHistoryRewardPercentiles verifies that reward percentiles reflect the
// effective priority fee per gas of ETH-typed txs in the block.
func TestFeeHistoryRewardPercentiles(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	ethTx := testutil.BuildEthTx(t, c.Repo().ChainID(), sender, 0, &recipient.Address)
	require.NoError(t, c.MintBlock(ethTx))

	ts := testutil.NewTestServer(t, fees.New(c.Repo(), 100, &testchain.DefaultForkConfig))

	result := testutil.Call(t, ts, "eth_feeHistory", []any{1, "latest", []float64{50}})
	var fh map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &fh))

	var rewards [][]*hexutil.Big
	require.NoError(t, json.Unmarshal(fh["reward"], &rewards))
	require.Len(t, rewards, 1)
	require.Len(t, rewards[0], 1)
	assert.True(t, rewards[0][0].ToInt().Sign() > 0,
		"ETH-typed tx priority fee must be reflected in reward")
}

// TestFeeHistoryRewardEthOnly verifies that reward percentiles, like gasUsedRatio,
// count only TypeEthDynamicFee gas and ignore VeChain legacy tx priority fees.
func TestFeeHistoryRewardEthOnly(t *testing.T) {
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	// Mint a block containing only a VeChain legacy tx. It pays a priority fee at
	// the block level but is not ETH-typed, so it must not affect reward.
	vcTx := testutil.BuildVcTx(t, c, sender, &recipient.Address)
	require.NoError(t, c.MintBlock(vcTx))

	ts := testutil.NewTestServer(t, fees.New(c.Repo(), 100, &testchain.DefaultForkConfig))

	result := testutil.Call(t, ts, "eth_feeHistory", []any{1, "latest", []float64{50}})
	var fh map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &fh))

	var rewards [][]*hexutil.Big
	require.NoError(t, json.Unmarshal(fh["reward"], &rewards))
	require.Len(t, rewards, 1)
	require.Len(t, rewards[0], 1)
	assert.Equal(t, 0, rewards[0][0].ToInt().Sign(),
		"VeChain legacy tx priority fee must not contribute to reward")
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
