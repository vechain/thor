// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thor"
)

func TestSign(t *testing.T) {
	// Generate a new private key for testing
	pk, err := crypto.GenerateKey()
	assert.NoError(t, err)

	txs := []*Transaction{
		new(LegacyBuilder).Build(),
		new(DynFeeBuilder).Build(),
	}

	for _, tx := range txs {
		// Sign the transaction
		signedTx, err := Sign(tx, pk)
		assert.NoError(t, err)

		// Verify the transaction was signed
		assert.NotNil(t, signedTx)

		// Verify address from Origin
		addr, err := signedTx.Origin()
		require.NoError(t, err)
		assert.Equal(t, thor.Address(crypto.PubkeyToAddress(pk.PublicKey)), addr)

		// Verify the delegator
		delegator, err := signedTx.Delegator()
		require.NoError(t, err)
		assert.Nil(t, delegator)
	}
}

func TestSignDelegated(t *testing.T) {
	// Generate a new private key for testing
	delegatorPK, err := crypto.GenerateKey()
	assert.NoError(t, err)

	originPK, err := crypto.GenerateKey()
	assert.NoError(t, err)

	txs := []*Transaction{
		new(LegacyBuilder).Build(),
		new(DynFeeBuilder).Build(),
	}

	for _, tx := range txs {
		// Feature not enabled
		signedTx, err := SignDelegated(tx, originPK, delegatorPK)
		assert.ErrorContains(t, err, "transaction delegated feature is not enabled")
		assert.Nil(t, signedTx)

		// enable the feature
		var features Features
		features.SetDelegated(true)
		tx = new(LegacyBuilder).Features(features).Build()

		// Sign the transaction as a delegator
		signedTx, err = SignDelegated(tx, originPK, delegatorPK)
		assert.NoError(t, err)
		assert.NotNil(t, signedTx)

		// Verify address from Origin
		origin, err := signedTx.Origin()
		require.NoError(t, err)
		assert.Equal(t, thor.Address(crypto.PubkeyToAddress(originPK.PublicKey)), origin)

		// Verify the delegator
		delegator, err := signedTx.Delegator()
		require.NoError(t, err)
		assert.Equal(t, thor.Address(crypto.PubkeyToAddress(delegatorPK.PublicKey)), *delegator)
	}
}
