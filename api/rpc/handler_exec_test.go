// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
)

// --- happy path ----------------------------------------------------------

func TestHandle_Call_EmptyCall_ReturnsHex0x(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	s.cfg.CallGasLimit = 5_000_000

	devAddr := genesis.DevAccounts()[0].Address
	// Call to an unrelated EOA with no data — legal, returns empty bytes.
	body := `{"jsonrpc":"2.0","method":"eth_call","params":[{"from":"` + devAddr.String() + `","to":"0x000000000000000000000000000000000000dead","data":"0x"},"latest"],"id":1}`

	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))
	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "0x", got, "call against EOA with no data returns 0x")
}

// --- access list rejection ----------------------------------------------

func TestHandle_Call_NonEmptyAccessList_Rejected(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	s.cfg.CallGasLimit = 1_000_000

	body := `{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x000000000000000000000000000000000000dead","accessList":[{"address":"0x0000000000000000000000000000000000000001","storageKeys":[]}]},"latest"],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeServerError, e.Code)
	assertReason(t, e, ReasonAccessListNotSupported)
}

// --- state overrides rejection ------------------------------------------

func TestHandle_Call_StateOverridesRejected(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	s.cfg.CallGasLimit = 1_000_000

	body := `{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x000000000000000000000000000000000000dead","stateOverrides":{"0x0000000000000000000000000000000000000001":{"balance":"0xff"}}},"latest"],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assertReason(t, e, ReasonStateOverridesNotSupported)
}

// --- gas cap enforcement ------------------------------------------------

func TestHandle_Call_GasAboveCap_Rejected(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	s.cfg.CallGasLimit = 100_000

	body := `{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x000000000000000000000000000000000000dead","gas":"0xfffff"},"latest"],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assertReason(t, e, ReasonGasCapExceeded)
}

// --- mutual-exclusion gasPrice / maxFeePerGas ---------------------------

func TestHandle_Call_GasPriceAndMaxFee_Exclusive(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	s.cfg.CallGasLimit = 1_000_000

	body := `{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x000000000000000000000000000000000000dead","gasPrice":"0x1","maxFeePerGas":"0x2"},"latest"],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

// --- data/input synonym handling ----------------------------------------

func TestHandle_Call_DataAndInputMismatch_Rejected(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	s.cfg.CallGasLimit = 1_000_000

	body := `{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x000000000000000000000000000000000000dead","data":"0x11","input":"0x22"},"latest"],"id":1}`
	_, errField := postJSON(t, s, body)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

// --- estimateGas --------------------------------------------------------

func TestHandle_EstimateGas_Call(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	s.cfg.CallGasLimit = 5_000_000

	devAddr := genesis.DevAccounts()[0].Address
	body := `{"jsonrpc":"2.0","method":"eth_estimateGas","params":[{"from":"` + devAddr.String() + `","to":"0x000000000000000000000000000000000000dead","data":"0x"},"latest"],"id":1}`
	result, errField := postJSON(t, s, body)
	require.Empty(t, errField, "err: %s", string(errField))

	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	// Empty-data transfer to a non-contract: EVM consumes 0, intrinsic is
	// TxGas (5000) + ClauseGas (16000) = 21000. No binary search; the answer
	// is the sum of the one execution + intrinsic.
	assert.Equal(t, "0x5208", got, "expected intrinsic-only 21000")
}

// --- decodeRevertReason unit test ---------------------------------------

func TestDecodeRevertReason_ErrorString(t *testing.T) {
	// ABI-encoded Error("hello"): selector 08c379a0 | offset=0x20 | len=5 | "hello" padded
	data := []byte{0x08, 0xc3, 0x79, 0xa0}
	data = append(data, make([]byte, 31)...)
	data = append(data, 0x20)
	data = append(data, make([]byte, 31)...)
	data = append(data, 0x05)
	data = append(data, []byte("hello")...)
	data = append(data, make([]byte, 32-5)...)

	msg := decodeRevertReason(data, nil)
	assert.Contains(t, msg, "hello")
}

func TestDecodeRevertReason_Panic(t *testing.T) {
	data := []byte{0x4e, 0x48, 0x7b, 0x71}
	msg := decodeRevertReason(data, nil)
	assert.Contains(t, msg, "panic")
}

func TestDecodeRevertReason_Fallback(t *testing.T) {
	msg := decodeRevertReason(nil, assertErr{msg: "boom"})
	assert.Contains(t, msg, "boom")
}

type assertErr struct{ msg string }

func (a assertErr) Error() string { return a.msg }
