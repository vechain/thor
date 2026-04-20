// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// TestResolveEthDynamicFee_EmptyAccessList_OK confirms a 0x02 tx with no
// access list passes resolution.
func TestResolveEthDynamicFee_EmptyAccessList_OK(t *testing.T) {
	to := thor.BytesToAddress([]byte("to"))
	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(big.NewInt(1)).
		EthTo(&to).
		EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(1000)).
		MaxPriorityFeePerGas(big.NewInt(100)).
		Gas(21000).
		Nonce(0).
		Build()
	signed := tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)

	_, err := runtime.ResolveTransaction(signed)
	assert.NoError(t, err)
}

// TestResolveEthDynamicFee_NonEmptyAccessList_Rejected verifies that a non-empty
// access list is rejected at the resolve stage with a clear error. Decode
// succeeds (so hashes stay bit-exact with Ethereum) but execution refuses.
func TestResolveEthDynamicFee_NonEmptyAccessList_Rejected(t *testing.T) {
	to := thor.BytesToAddress([]byte("to"))
	trx := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(big.NewInt(1)).
		EthTo(&to).
		EthValue(big.NewInt(0)).
		MaxFeePerGas(big.NewInt(1000)).
		MaxPriorityFeePerGas(big.NewInt(100)).
		Gas(21000).
		Nonce(0).
		AccessList(tx.AccessList{
			{Address: thor.Address{0x01}, StorageKeys: []thor.Bytes32{{0x02}}},
		}).
		Build()
	signed := tx.MustSign(trx, genesis.DevAccounts()[0].PrivateKey)

	_, err := runtime.ResolveTransaction(signed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access list")
}