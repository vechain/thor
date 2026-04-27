// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Spec 3 integration: sequential-nonce semantics + eth-style CREATE address
// derivation for 0x02 txs past INTERSTELLAR. Covers the happy path (nonce
// advances 0→1→2 across mined blocks), admit-time nonce_too_low rejection,
// future-nonce parking in the non-executable queue, and `contractAddress` =
// `keccak(rlp(sender, nonce))[12:]` for top-level deployment.

package tests

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// decodeHexUint parses a `"0xNN"` JSON string into uint64.
func decodeHexUint(t *testing.T, raw json.RawMessage) uint64 {
	t.Helper()
	var s string
	require.NoError(t, json.Unmarshal(raw, &s))
	n, ok := new(big.Int).SetString(strings.TrimPrefix(s, "0x"), 16)
	require.True(t, ok)
	return n.Uint64()
}

func TestEthNonce_SequentialMining(t *testing.T) {
	srv, tc, _ := newE2EServer(t)
	chainID := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	sender := genesis.DevAccounts()[0]
	to := thor.BytesToAddress([]byte("dst"))

	// Fresh account: eth_getTransactionCount returns 0.
	countRaw, errField := callRPC(t, srv, "eth_getTransactionCount", sender.Address.String(), "latest")
	require.Empty(t, errField)
	assert.Equal(t, uint64(0), decodeHexUint(t, countRaw))

	// Three sequential 0x02 txs (nonce 0, 1, 2). Each must admit, mine, and
	// advance the on-chain nonce.
	for want := uint64(0); want <= 2; want++ {
		trx := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
			ChainID(chainID).
			EthTo(&to).
			EthValue(big.NewInt(0)).
			MaxFeePerGas(big.NewInt(10_000_000_000_000)).
			MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
			Gas(21000).
			Nonce(want).
			Build(), sender.PrivateKey)

		raw, err := trx.MarshalBinary()
		require.NoError(t, err)
		_, errField := callRPC(t, srv, "eth_sendRawTransaction", "0x"+hex.EncodeToString(raw))
		require.Empty(t, errField, "nonce %d admit err: %s", want, string(errField))
		require.NoError(t, tc.MintBlock(trx))

		countRaw, errField := callRPC(t, srv, "eth_getTransactionCount", sender.Address.String(), "latest")
		require.Empty(t, errField)
		assert.Equal(t, want+1, decodeHexUint(t, countRaw), "nonce after tx %d", want)
	}
}

func TestEthNonce_TooLowIsRejected(t *testing.T) {
	srv, tc, _ := newE2EServer(t)
	chainID := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	sender := genesis.DevAccounts()[0]
	to := thor.BytesToAddress([]byte("dst"))

	// Mine nonce=0 first.
	trx := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).EthTo(&to).EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(10_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(21000).Nonce(0).Build(), sender.PrivateKey)
	raw, _ := trx.MarshalBinary()
	_, errField := callRPC(t, srv, "eth_sendRawTransaction", "0x"+hex.EncodeToString(raw))
	require.Empty(t, errField)
	require.NoError(t, tc.MintBlock(trx))

	// Fresh tx with same nonce=0 but different value so its hash does not
	// match the mined tx (the pool's ContainsHash short-circuit would
	// otherwise silently accept a strict byte-for-byte replay as idempotent).
	// After mining, state.Nonce == 1, so tx.Nonce=0 is "too low" at admit.
	low := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).EthTo(&to).EthValue(big.NewInt(1)).
		MaxFeePerGas(big.NewInt(10_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(21000).Nonce(0).Build(), sender.PrivateKey)
	lowRaw, _ := low.MarshalBinary()
	_, errField = callRPC(t, srv, "eth_sendRawTransaction", "0x"+hex.EncodeToString(lowRaw))
	require.NotEmpty(t, errField)
	var e map[string]any
	require.NoError(t, json.Unmarshal(errField, &e))
	assert.Equal(t, float64(-32000), e["code"])
	data, _ := e["data"].(map[string]any)
	assert.Equal(t, "nonce_too_low", data["reason"])
}

func TestEthNonce_FutureNonceQueued(t *testing.T) {
	srv, tc, _ := newE2EServer(t)
	chainID := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	sender := genesis.DevAccounts()[0]
	to := thor.BytesToAddress([]byte("dst"))

	// Sending nonce=3 on a fresh account via pool.AddLocal (which is what
	// eth_sendRawTransaction uses) returns the canonical txid — the tx is
	// parked in the non-executable queue until state nonce catches up,
	// matching go-ethereum's "queued" pool semantics.
	trx := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).EthTo(&to).EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(10_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(21000).Nonce(3).Build(), sender.PrivateKey)
	raw, _ := trx.MarshalBinary()
	resultField, errField := callRPC(t, srv, "eth_sendRawTransaction", "0x"+hex.EncodeToString(raw))
	require.Empty(t, errField, "future-nonce under AddLocal must be accepted, not rejected")
	var txid string
	require.NoError(t, json.Unmarshal(resultField, &txid))
	assert.Equal(t, trx.CanonicalTxID().String(), txid)
}

func TestEthNonce_EthStyleCreateAddress(t *testing.T) {
	srv, tc, _ := newE2EServer(t)
	chainID := new(big.Int).SetUint64(thor.ChainID(tc.Repo().GenesisBlock().Header().ID()))
	sender := genesis.DevAccounts()[0]

	// Minimal init-code: returns 1 byte (0x00) as runtime code.
	//   PUSH1 0x01  PUSH1 0x00  MSTORE  PUSH1 0x01  PUSH1 0x1f  RETURN
	initCode := common.Hex2Bytes("6001600052600160f3")

	// Nonce=0 → eth address = keccak(rlp(sender, 0))[12:].
	expected := common.Address(crypto.CreateAddress(common.Address(sender.Address), 0))

	trx := tx.MustSign(tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		EthTo(nil). // nil To signals CREATE
		EthData(initCode).
		EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(10_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(300_000).
		Nonce(0).
		Build(), sender.PrivateKey)
	raw, err := trx.MarshalBinary()
	require.NoError(t, err)

	_, errField := callRPC(t, srv, "eth_sendRawTransaction", "0x"+hex.EncodeToString(raw))
	require.Empty(t, errField, "create admit err: %s", string(errField))
	require.NoError(t, tc.MintBlock(trx))

	got, errField := callRPC(t, srv, "eth_getTransactionReceipt", trx.CanonicalTxID().String())
	require.Empty(t, errField)
	var rcpt map[string]any
	require.NoError(t, json.Unmarshal(got, &rcpt))
	assert.Equal(t, "0x1", rcpt["status"], "CREATE must succeed; receipt: %+v", rcpt)

	addrStr, _ := rcpt["contractAddress"].(string)
	require.NotEmpty(t, addrStr, "receipt must carry contractAddress")
	var got0 thor.Address
	require.NoError(t, got0.UnmarshalJSON([]byte(`"`+addrStr+`"`)))
	assert.Equal(t, strings.ToLower(expected.Hex()), strings.ToLower(got0.String()),
		"0x02 CREATE address must be keccak(rlp(origin, nonce))[12:] per eth")

	// Account nonce of sender is now 1 (pre-bump persisted).
	countRaw, _ := callRPC(t, srv, "eth_getTransactionCount", sender.Address.String(), "latest")
	assert.Equal(t, uint64(1), decodeHexUint(t, countRaw))
}
