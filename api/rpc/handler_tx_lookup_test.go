// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// mintAndPack returns a server whose txpool admitted a single 0x02 tx and
// whose chain has been extended one block past that tx. Used by the lookup
// tests below.
func mintAndPack(t *testing.T) (*Server, thor.Bytes32) {
	t.Helper()
	s, tc := newTestServerWithPool(t)

	chainID := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	to := thor.BytesToAddress([]byte("look"))
	raw := rawEth02(t, chainID, 0, &to, 10_000_000_000_000, 1_000_000_000)

	trx := new(tx.Transaction)
	require.NoError(t, trx.UnmarshalBinary(raw))

	// Mint the block containing the tx. MintBlock takes the tx args directly.
	require.NoError(t, tc.MintBlock(trx))
	return s, trx.CanonicalTxID()
}

// --- eth_getTransactionByHash -------------------------------------------

func TestHandle_GetTransactionByHash_Mined(t *testing.T) {
	s, txid := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["` + txid.String() + `"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, txid.String(), got["hash"])
	assert.Equal(t, "0x2", got["type"])
	require.NotNil(t, got["blockHash"])
	require.NotNil(t, got["blockNumber"])
}

func TestHandle_GetTransactionByHash_NotFound_ReturnsNull(t *testing.T) {
	s, _ := newTestServerWithPool(t)

	body := `{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0x` + hex.EncodeToString(make([]byte, 32)) + `"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))
	assert.Equal(t, json.RawMessage("null"), result)
}

func TestHandle_GetTransactionByHash_BadParam(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	_, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":[],"id":1}`)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

// --- eth_getTransactionReceipt ------------------------------------------

func TestHandle_GetTransactionReceipt_Mined(t *testing.T) {
	s, txid := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getTransactionReceipt","params":["` + txid.String() + `"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, txid.String(), got["transactionHash"])
	assert.Equal(t, "0x2", got["type"])
	assert.Equal(t, "0x1", got["status"], "non-reverted receipt has status=1")
	// logsBloom is always 256 zero bytes.
	bloom, _ := got["logsBloom"].(string)
	assert.Len(t, bloom, 2+2*256)
	assert.Equal(t, 256*2, countHexChar(bloom[2:], '0'))
}

func TestHandle_GetTransactionReceipt_NotFound(t *testing.T) {
	s, _ := newTestServerWithPool(t)

	body := `{"jsonrpc":"2.0","method":"eth_getTransactionReceipt","params":["0x` + hex.EncodeToString(make([]byte, 32)) + `"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	assert.Equal(t, json.RawMessage("null"), result)
}

// --- eth_getTransactionByBlockNumberAndIndex ----------------------------

func TestHandle_GetTransactionByBlockNumberAndIndex(t *testing.T) {
	s, txid := mintAndPack(t)

	// The minted tx is at block 1 (genesis=0, minted=1), index 0.
	body := `{"jsonrpc":"2.0","method":"eth_getTransactionByBlockNumberAndIndex","params":["0x1","0x0"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, txid.String(), got["hash"])
}

func TestHandle_GetTransactionByBlockNumberAndIndex_OutOfRange(t *testing.T) {
	s, _ := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getTransactionByBlockNumberAndIndex","params":["0x1","0x9"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	assert.Equal(t, json.RawMessage("null"), result, "out-of-range index returns null")
}

// --- eth_getTransactionByBlockHashAndIndex ------------------------------

func TestHandle_GetTransactionByBlockHashAndIndex(t *testing.T) {
	s, txid := mintAndPack(t)

	// Find block 1's id.
	best := s.repo.BestBlockSummary()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getTransactionByBlockHashAndIndex","params":["%s","0x0"],"id":1}`, best.Header.ID().String())
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got map[string]any
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, txid.String(), got["hash"])
}

// --- helpers ------------------------------------------------------------

func countHexChar(s string, ch byte) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ch {
			n++
		}
	}
	return n
}

var _ = genesis.DevAccounts // keep import if future tests need it
