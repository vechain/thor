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

	"github.com/vechain/thor/v2/rpc/fees"
	"github.com/vechain/thor/v2/rpc/testutil"
)

func TestFeesHandler(t *testing.T) {
	fx := testutil.NewChainFixture(t)
	ts := testutil.NewMinimalServer(t, fees.New(fx.Chain.Repo(), 100))

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
}
