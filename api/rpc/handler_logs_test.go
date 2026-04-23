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

// --- empty logdb --------------------------------------------------------

func TestHandle_GetLogs_EmptyResult(t *testing.T) {
	s, _ := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0x0","toBlock":"latest"}],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got []any
	require.NoError(t, json.Unmarshal(result, &got))
	// The minted 0x02 tx with to=address doesn't emit any logs, so the
	// result is an empty array — JSON must emit [], not null.
	assert.Equal(t, 0, len(got))
}

// --- range cap ---------------------------------------------------------

func TestHandle_GetLogs_RangeTooLarge(t *testing.T) {
	s, _ := mintAndPack(t)
	s.cfg.APIBacktraceLimit = 0 // explicitly zero -> treat as "no cap"

	// First, verify no cap accepts a small range.
	body := `{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0x0","toBlock":"0x1"}],"id":1}`
	_, errField := postJSON(t, s, body)
	require.Empty(t, errField)

	// Now set a tight cap and re-run -> should reject.
	s.cfg.APIBacktraceLimit = 1
	body = `{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0x0","toBlock":"0x1"}],"id":1}`
	_, errField = postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assertReason(t, e, ReasonLogRangeTooLarge)
}

// --- block-hash filter -------------------------------------------------

func TestHandle_GetLogs_BlockHash_Canonical(t *testing.T) {
	s, _ := mintAndPack(t)

	best := s.repo.BestBlockSummary()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"blockHash":"%s"}],"id":1}`, best.Header.ID().String())
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))
	var got []any
	require.NoError(t, json.Unmarshal(result, &got))
	// No events in the minted block, but the call must succeed.
	assert.Equal(t, 0, len(got))
}

func TestHandle_GetLogs_BlockHash_XorBlockRange(t *testing.T) {
	s, _ := mintAndPack(t)

	best := s.repo.BestBlockSummary()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"blockHash":"%s","fromBlock":"0x0"}],"id":1}`, best.Header.ID().String())
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

// --- topic cap ---------------------------------------------------------

func TestHandle_GetLogs_TooManyTopics(t *testing.T) {
	s, _ := mintAndPack(t)

	body := `{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0x0","toBlock":"latest","topics":[null,null,null,null,null]}],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

// --- combineTopics unit ------------------------------------------------

func TestCombineTopics(t *testing.T) {
	// Empty input => single empty tuple.
	got := combineTopics(nil)
	require.Len(t, got, 1)
	assert.Equal(t, 0, len(got[0]))
}

// --- cross-product via JSON filter ------------------------------------

func TestHandle_GetLogs_AddressArray_SmokeThroughFilter(t *testing.T) {
	s, _ := mintAndPack(t)
	body := `{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0x0","toBlock":"latest","address":["0x0000000000000000000000000000000000000001","0x0000000000000000000000000000000000000002"]}],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))
	var got []any
	require.NoError(t, json.Unmarshal(result, &got))
	// Still zero events (no contracts deployed / no emits in the minted
	// block); the assertion is that the cross-product filter compiled and
	// the logdb round-tripped without error.
	assert.Equal(t, 0, len(got))
}
