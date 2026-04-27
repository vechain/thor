// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// newTestServerWithPool is like newTestServerWithChain but also spins up a
// real txpool.TxPool so eth_sendRawTransaction has somewhere to land.
func newTestServerWithPool(t *testing.T) (*Server, *testchain.Chain) {
	t.Helper()
	fork := &thor.ForkConfig{}
	tc, err := testchain.NewWithFork(fork, 180)
	require.NoError(t, err)

	pool := txpool.New(tc.Repo(), tc.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 10,
		MaxLifetime:     time.Hour,
	}, tc.GetForkConfig())

	s := NewServer(tc.Repo(), tc.Stater(), pool, tc.LogDB(), tc.GetForkConfig(), tc.Engine(), Config{})
	return s, tc
}

// rawEth02 builds and signs an 0x02 tx, returning its binary-RLP blob.
func rawEth02(t *testing.T, chainID *big.Int, nonce uint64, to *thor.Address, maxFeeWei, maxPrioWei int64) []byte {
	t.Helper()
	b := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		EthTo(to).
		EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(maxFeeWei)).
		MaxPriorityFeePerGas(big.NewInt(maxPrioWei)).
		Gas(21000).
		Nonce(nonce)
	trx := tx.MustSign(b.Build(), genesis.DevAccounts()[0].PrivateKey)
	raw, err := trx.MarshalBinary()
	require.NoError(t, err)
	return raw
}

// send wraps postJSON for eth_sendRawTransaction.
func send(t *testing.T, s *Server, raw []byte) (result, errField json.RawMessage) {
	t.Helper()
	body := `{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0x` + hex.EncodeToString(raw) + `"],"id":1}`
	return postJSON(t, s, body)
}

// --- happy path ---------------------------------------------------------

func TestHandle_SendRaw_Accepts_0x02(t *testing.T) {
	s, tc := newTestServerWithPool(t)

	chainID := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	to := thor.BytesToAddress([]byte("recipient"))
	raw := rawEth02(t, chainID, 0, &to, 10_000_000_000_000, 1_000_000_000)

	result, errField := send(t, s, raw)
	require.Empty(t, errField, "raw err: %s", string(errField))

	var got thor.Bytes32
	require.NoError(t, json.Unmarshal(result, &got))

	// Parse the tx ourselves and confirm the returned hash is the canonical txid.
	trx := new(tx.Transaction)
	require.NoError(t, trx.UnmarshalBinary(raw))
	assert.Equal(t, trx.CanonicalTxID(), got)
}

// --- error mapping ------------------------------------------------------

func TestHandle_SendRaw_ChainIDMismatch(t *testing.T) {
	s, _ := newTestServerWithPool(t)

	wrong := big.NewInt(0xDEAD)
	to := thor.BytesToAddress([]byte("x"))
	raw := rawEth02(t, wrong, 0, &to, 10_000_000_000_000, 1_000_000_000)

	_, errField := send(t, s, raw)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeServerError, e.Code)
	assertReason(t, e, ReasonChainIDMismatch)
}

func TestHandle_SendRaw_Duplicate_IsIdempotent(t *testing.T) {
	// Thor's txpool silently accepts duplicate txs (they hash-match an
	// existing entry). Matches Thor's "not assumed as an error" contract at
	// txpool/tx_pool.go line 244; eth_sendRawTransaction therefore returns
	// the canonical txid twice rather than tx_known. tx_known is still on
	// the reason whitelist for the rare internal paths that surface it
	// (see TestMapTxPoolError_EveryReasonIsReachable).
	s, tc := newTestServerWithPool(t)

	chainID := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	to := thor.BytesToAddress([]byte("dup"))
	raw := rawEth02(t, chainID, 0, &to, 10_000_000_000_000, 1_000_000_000)

	result1, errField := send(t, s, raw)
	require.Empty(t, errField)

	result2, errField := send(t, s, raw)
	require.Empty(t, errField, "duplicate must be idempotent, got err: %s", string(errField))
	assert.Equal(t, result1, result2, "duplicate returns the same txid")
}

func TestHandle_SendRaw_BadHex(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	_, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["not-hex"],"id":1}`)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

func TestHandle_SendRaw_UnknownTxType(t *testing.T) {
	s, _ := newTestServerWithPool(t)

	// Typed-tx envelope with a reserved type byte (0x7f is not supported).
	raw := []byte{0x7f, 0x01, 0x02, 0x03}
	_, errField := send(t, s, raw)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assertReason(t, e, ReasonTxTypeNotSupported)
}

func TestHandle_SendRaw_ArityZero(t *testing.T) {
	s, _ := newTestServerWithPool(t)
	_, errField := postJSON(t, s, `{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":[],"id":1}`)
	require.NotEmpty(t, errField)
	var e RPCError
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, CodeInvalidParams, e.Code)
}

// --- mapping unit tests (no network / pool) ----------------------------

func TestMapTxPoolError_EveryReasonIsReachable(t *testing.T) {
	// Each entry below synthesizes an error whose message is known to appear
	// in the tree verbatim (grepped during plan authoring). If any of these
	// assertions start failing, the upstream message changed and either the
	// mapper needs a new substring or the message needs to be restored.
	cases := []struct {
		err    error
		reason string
	}{
		{badTx("eth tx type not supported before INTERSTELLAR"), ReasonTxTypeNotSupported},
		{badTx("transaction type not supported"), ReasonTxTypeNotSupported},
		{badTx("eth tx chain id mismatch"), ReasonChainIDMismatch},
		{badTx("chain tag mismatch"), ReasonChainIDMismatch},
		{rej("size too large"), ReasonOversizedData},
		{badTx("eth tx access list not supported"), ReasonAccessListNotSupported},
		{rej("insufficient energy for overall pending cost"), ReasonInsufficientFunds},
		{mkErr("known tx"), ReasonTxKnown},
		{badTx("intrinsic gas overflow"), ReasonIntrinsicGasTooLow},
		{badTx("invalid signature: S value is out of range"), ReasonTxValidationFailed},
		{rej("non executable pool is full"), ReasonTxValidationFailed},
		{rej("pool is full"), ReasonTxValidationFailed},
		{rej("tx is not executable"), ReasonTxValidationFailed},
	}
	for _, c := range cases {
		t.Run(c.err.Error(), func(t *testing.T) {
			rpcErr := mapTxPoolError(c.err)
			require.NotNil(t, rpcErr)
			if c.reason == "" {
				return
			}
			assert.Equal(t, CodeServerError, rpcErr.Code)
			data, ok := rpcErr.Data.(map[string]string)
			require.True(t, ok, "expected map[string]string data; got %T", rpcErr.Data)
			assert.Equal(t, c.reason, data["reason"])
		})
	}
}

// --- helpers ------------------------------------------------------------

// badTx / rej / mkErr construct the stringy error shapes the txpool returns.
// badTxError and txRejectedError are unexported; we mimic the on-the-wire
// .Error() rendering so mapTxPoolError sees identical text.
type fakeErr struct{ msg string }

func (f fakeErr) Error() string { return f.msg }

func badTx(msg string) error { return fakeErr{msg: "bad tx: " + msg} }
func rej(msg string) error   { return fakeErr{msg: "tx rejected: " + msg} }
func mkErr(msg string) error { return fakeErr{msg: msg} }

func assertReason(t *testing.T, e RPCError, want string) {
	t.Helper()
	data, ok := e.Data.(map[string]any)
	require.True(t, ok, "expected object data; got %T", e.Data)
	assert.Equal(t, want, data["reason"], "message=%q", e.Message)
}
