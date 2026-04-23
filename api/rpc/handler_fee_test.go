// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandle_GasPrice_ReturnsPositive(t *testing.T) {
	s, _ := newTestServerWithPool(t)

	result, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_gasPrice","id":1}`)
	require.Empty(t, errField, "err: %s", string(errField))
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	n, ok := new(big.Int).SetString(strings.TrimPrefix(got, "0x"), 16)
	require.True(t, ok)
	assert.Equal(t, 1, n.Sign(), "gasPrice must be positive")
}

func TestHandle_MaxPriorityFeePerGas(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	s.cfg.PriorityIncreasePercentage = 5

	result, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_maxPriorityFeePerGas","id":1}`)
	require.Empty(t, errField)
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	n, ok := new(big.Int).SetString(strings.TrimPrefix(got, "0x"), 16)
	require.True(t, ok)
	assert.Equal(t, 1, n.Sign(), "priority fee must be positive under non-zero percentage")
}

func TestHandle_FeeHistory_HappyPath(t *testing.T) {
	s, _ := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_feeHistory","params":["0x1","latest",[25,75]],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got feeHistoryResult
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, uint64(1), uint64(got.OldestBlock))
	// baseFeePerGas has blockCount+1 entries.
	assert.Len(t, got.BaseFeePerGas, 2)
	assert.Len(t, got.GasUsedRatio, 1)
	require.NotNil(t, got.Reward)
	require.Len(t, got.Reward, 1)
	assert.Len(t, got.Reward[0], 2)
}

func TestHandle_FeeHistory_BlockCountZero_Rejected(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	body := `{"jsonrpc":"2.0","method":"eth_feeHistory","params":["0x0","latest"],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

func TestHandle_FeeHistory_InvalidPercentile(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	body := `{"jsonrpc":"2.0","method":"eth_feeHistory","params":["0x1","latest",[150]],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

func TestHandle_FeeHistory_Descending_Rejected(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	body := `{"jsonrpc":"2.0","method":"eth_feeHistory","params":["0x1","latest",[75,25]],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

func TestHandle_FeeHistory_NoPercentiles_OmitsReward(t *testing.T) {
	s, _ := mintAndPack(t)
	body := `{"jsonrpc":"2.0","method":"eth_feeHistory","params":["0x1","latest"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)

	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &env))
	_, hasReward := env["reward"]
	assert.False(t, hasReward, "reward field omitted when no percentiles supplied")
}
