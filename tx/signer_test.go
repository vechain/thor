// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/stretchr/testify/assert"
)

func TestSignTx(t *testing.T) {
	// Generate a new private key for testing
	pk, err := crypto.GenerateKey()
	assert.NoError(t, err)

	tx := new(Builder).Build()

	// Sign the transaction
	signedTx, err := Sign(tx, pk)
	assert.NoError(t, err)

	// Verify the transaction was signed
	assert.NotNil(t, signedTx)
}

func TestDelegatorSignTx(t *testing.T) {
	// Generate a new private key for testing
	delegatorPK, err := crypto.GenerateKey()
	assert.NoError(t, err)

	originPK, err := crypto.GenerateKey()
	assert.NoError(t, err)

	tx := new(Builder).Build()

	// Feature not enabled
	signedTx, err := SignDelegator(tx, originPK, delegatorPK)
	assert.ErrorContains(t, err, "transaction delegated feature is not enabled")
	assert.Nil(t, signedTx)

	// enable the feature
	var features Features
	features.SetDelegated(true)

	// tx is already signed
	tx = new(Builder).Features(features).Build()
	signedTx = MustSign(tx, originPK)
	signedTx, err = SignDelegator(signedTx, originPK, delegatorPK)
	assert.ErrorIs(t, err, secp256k1.ErrInvalidSignatureLen)
	assert.Nil(t, signedTx)

	// Sign the transaction as a delegator
	signedTx, err = SignDelegator(tx, originPK, delegatorPK)
	assert.NoError(t, err)
	assert.NotNil(t, signedTx)
}
