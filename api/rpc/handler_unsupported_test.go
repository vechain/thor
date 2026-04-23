// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandle_CreateAccessList_EmptyAndZero(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	body := `{"jsonrpc":"2.0","method":"eth_createAccessList","params":[{}],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))
	al, ok := got["accessList"].([]any)
	require.True(t, ok)
	assert.Len(t, al, 0)
	assert.Equal(t, "0x0", got["gasUsed"])
}

func TestHandle_GetProof_MethodNotFound(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	body := `{"jsonrpc":"2.0","method":"eth_getProof","params":[],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeMethodNotFound, e.Code)
}

func TestHandle_UncleCount_Zero(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	for _, m := range []string{"eth_getUncleCountByBlockHash", "eth_getUncleCountByBlockNumber"} {
		body := `{"jsonrpc":"2.0","method":"` + m + `","params":[null],"id":1}`
		result, errField := postJSON(t, s, body)
		require.Empty(t, errField, "method %s err: %s", m, string(errField))
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, "0x0", got)
	}
}

func TestHandle_UncleByIndex_Null(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	for _, m := range []string{"eth_getUncleByBlockHashAndIndex", "eth_getUncleByBlockNumberAndIndex"} {
		body := `{"jsonrpc":"2.0","method":"` + m + `","params":[null,"0x0"],"id":1}`
		result, errField := postJSON(t, s, body)
		require.Empty(t, errField)
		assert.Equal(t, json.RawMessage("null"), result)
	}
}
