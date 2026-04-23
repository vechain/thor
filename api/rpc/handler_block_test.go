// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- eth_getBlockByNumber / eth_getBlockByHash -------------------------

func TestHandle_GetBlockByNumber_HashesOnly(t *testing.T) {
	s, txid := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x1",false],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x1", got["number"])
	assert.Contains(t, got, "hash")
	assert.Contains(t, got, "parentHash")
	assert.Contains(t, got, "sha3Uncles")
	assert.Contains(t, got, "logsBloom")
	assert.Contains(t, got, "miner")

	txs, ok := got["transactions"].([]any)
	require.True(t, ok, "transactions must be an array of hashes")
	require.Len(t, txs, 1)
	assert.Equal(t, txid.String(), txs[0])
}

func TestHandle_GetBlockByNumber_FullTx(t *testing.T) {
	s, txid := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x1",true],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))

	txs, ok := got["transactions"].([]any)
	require.True(t, ok)
	require.Len(t, txs, 1)
	txObj, ok := txs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, txid.String(), txObj["hash"])
	assert.Equal(t, "0x2", txObj["type"])
}

func TestHandle_GetBlockByNumber_LatestTag(t *testing.T) {
	s, _ := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",false],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x1", got["number"])
}

func TestHandle_GetBlockByHash_Genesis(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	genesisID := s.repo.GenesisBlock().Header().ID()

	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["%s",false],"id":1}`, genesisID.String())
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)

	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x0", got["number"])
	assert.Equal(t, genesisID.String(), got["hash"])
}

func TestHandle_GetBlockByHash_Unknown_Null(t *testing.T) {
	s, _ := newTestServerWithPool(t)

	body := `{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0x0000000000000000000000000000000000000000000000000000000000000000",false],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	assert.Equal(t, json.RawMessage("null"), result)
}

// --- eth_getBlockTransactionCount --------------------------------------

func TestHandle_GetBlockTransactionCountByNumber(t *testing.T) {
	s, _ := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByNumber","params":["0x1"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x1", got)
}

func TestHandle_GetBlockTransactionCountByHash_Genesis(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	genesisID := s.repo.GenesisBlock().Header().ID()

	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByHash","params":["%s"],"id":1}`, genesisID.String())
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x0", got)
}

// --- param errors ------------------------------------------------------

func TestHandle_GetBlockByNumber_ArityError(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	_, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest"],"id":1}`)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}
