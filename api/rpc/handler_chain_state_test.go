// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

// newTestServerWithChain returns a Server bound to a fresh testchain. The
// dispatch table is snapshotted from globalHandlers (populated by init()),
// so handler_chain / handler_state are already registered.
func newTestServerWithChain(t *testing.T, fork *thor.ForkConfig) (*Server, *testchain.Chain) {
	t.Helper()
	if fork == nil {
		fork = &thor.ForkConfig{}
	}
	tc, err := testchain.NewWithFork(fork, 180)
	require.NoError(t, err)
	s := NewServer(tc.Repo(), tc.Stater(), nil, tc.LogDB(), tc.GetForkConfig(), tc.Engine(), Config{})
	return s, tc
}

// postJSON drives s.ServeHTTP with the given JSON body and returns the
// result OR the error.
func postJSON(t *testing.T, s *Server, body string) (result, errField json.RawMessage) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), "raw: %s", rec.Body.String())
	return env["result"], env["error"]
}

// --- eth_chainId ---------------------------------------------------------

func TestHandle_ChainID_PostInterstellar(t *testing.T) {
	// forkFromStart (all forks at 0) → post-INTERSTELLAR at every block.
	s, tc := newTestServerWithChain(t, &thor.ForkConfig{})

	result, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	require.Empty(t, errField)

	// Expected = hex of uint16BE(genesisID[30:32]).
	genesisID := tc.Repo().GenesisBlock().Header().ID()
	expected := new(big.Int).SetUint64(thor.ChainID(genesisID))

	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	actual, ok := new(big.Int).SetString(strings.TrimPrefix(got, "0x"), 16)
	require.True(t, ok)
	assert.Equal(t, 0, expected.Cmp(actual))
}

func TestHandle_ChainID_PreInterstellar(t *testing.T) {
	// Push INTERSTELLAR far in the future — all best blocks stay pre-fork.
	s, tc := newTestServerWithChain(t, &thor.ForkConfig{INTERSTELLAR: 1_000_000})

	result, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	require.Empty(t, errField)

	genesisID := tc.Repo().GenesisBlock().Header().ID()
	expected := new(big.Int).SetBytes(genesisID.Bytes())
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	actual, ok := new(big.Int).SetString(strings.TrimPrefix(got, "0x"), 16)
	require.True(t, ok)
	assert.Equal(t, 0, expected.Cmp(actual), "pre-INTERSTELLAR returns 32-byte genesis id")
}

// --- eth_blockNumber -----------------------------------------------------

func TestHandle_BlockNumber(t *testing.T) {
	s, tc := newTestServerWithChain(t, nil)
	require.NoError(t, tc.MintBlock())
	require.NoError(t, tc.MintBlock())

	result, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`)
	require.Empty(t, errField)

	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x2", got, "two minted blocks above genesis")
}

// --- eth_syncing ---------------------------------------------------------

func TestHandle_Syncing(t *testing.T) {
	s, _ := newTestServerWithChain(t, nil)
	result, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_syncing","id":1}`)
	require.Empty(t, errField)
	assert.Equal(t, json.RawMessage("false"), result)
}

// --- eth_getBalance ------------------------------------------------------

func TestHandle_GetBalance_DevAccount(t *testing.T) {
	s, _ := newTestServerWithChain(t, nil)

	devAddr := genesis.DevAccounts()[0].Address
	body := `{"jsonrpc":"2.0","method":"eth_getBalance","params":["` + devAddr.String() + `","latest"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "raw error: %s", string(errField))

	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	// Dev accounts are well-funded — balance must be positive.
	balHex := strings.TrimPrefix(got, "0x")
	require.NotEqual(t, "0", balHex, "dev account balance should be non-zero")
	n, ok := new(big.Int).SetString(balHex, 16)
	require.True(t, ok)
	assert.Equal(t, 1, n.Sign())
}

func TestHandle_GetBalance_UnknownAddress(t *testing.T) {
	s, _ := newTestServerWithChain(t, nil)

	body := `{"jsonrpc":"2.0","method":"eth_getBalance","params":["0x000000000000000000000000000000000000dead","latest"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x0", got)
}

func TestHandle_GetBalance_ParamErrors(t *testing.T) {
	s, _ := newTestServerWithChain(t, nil)

	_, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_getBalance","params":[],"id":1}`)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)

	_, errField = postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_getBalance","params":["notahex","latest"],"id":1}`)
	require.NotEmpty(t, errField)
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

// --- eth_getCode ---------------------------------------------------------

func TestHandle_GetCode_EmptyForEOA(t *testing.T) {
	s, _ := newTestServerWithChain(t, nil)
	devAddr := genesis.DevAccounts()[0].Address

	body := `{"jsonrpc":"2.0","method":"eth_getCode","params":["` + devAddr.String() + `","latest"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x", got, "EOA has no code")
}

// --- eth_getStorageAt ----------------------------------------------------

func TestHandle_GetStorageAt_ReturnsZero(t *testing.T) {
	s, _ := newTestServerWithChain(t, nil)
	devAddr := genesis.DevAccounts()[0].Address
	zeroKey := thor.Bytes32{}

	body := `{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["` + devAddr.String() + `","` + zeroKey.String() + `","latest"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField)
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, zeroKey.String(), got)
}

func TestHandle_GetStorageAt_ParamCount(t *testing.T) {
	s, _ := newTestServerWithChain(t, nil)
	_, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0x0000000000000000000000000000000000000000"],"id":1}`)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

// --- eth_getTransactionCount --------------------------------------------

func TestHandle_GetTransactionCount_AlwaysZero(t *testing.T) {
	s, _ := newTestServerWithChain(t, nil)
	devAddr := genesis.DevAccounts()[0].Address

	for _, tag := range []string{`"latest"`, `"pending"`, `"earliest"`, `"finalized"`, `"0x0"`} {
		body := `{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["` + devAddr.String() + `",` + tag + `],"id":1}`
		result, errField := postJSON(t, s, body)
		require.Empty(t, errField, "tag %s err: %s", tag, string(errField))
		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, "0x0", got, "tag %s must return 0x0 per spec D2", tag)
	}
}
