// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"crypto/ecdsa"
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

	txTypes := []Type{TypeLegacy, TypeDynamicFee}

	for _, txType := range txTypes {
		trx := NewBuilder(txType).Build()
		// Sign the transaction
		signedTx, err := Sign(trx, pk)
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

	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
		// Feature not enabled
		trx := NewBuilder(txType).Build()
		signedTx, err := SignDelegated(trx, originPK, delegatorPK)
		assert.ErrorContains(t, err, "transaction delegated feature is not enabled")
		assert.Nil(t, signedTx)

		// enable the feature
		var features Features
		features.SetDelegated(true)
		trx = NewBuilder(txType).Features(features).Build()

		// Sign the transaction as a delegator
		signedTx, err = SignDelegated(trx, originPK, delegatorPK)
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

func TestMustSign_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustSign should panic on error")
		}
	}()
	// Use a nil tx to force panic
	MustSign(nil, nil)
}

func TestMustSign_ReturnsSignedTx(t *testing.T) {
	pk, _ := crypto.GenerateKey()
	trx := NewBuilder(TypeLegacy).Build()
	signed := MustSign(trx, pk)
	assert.NotNil(t, signed)
}

func TestMustSignDelegated_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustSignDelegated should panic on error")
		}
	}()
	// Use a tx without delegated feature to force panic
	pk, _ := crypto.GenerateKey()
	MustSignDelegated(NewBuilder(TypeLegacy).Build(), pk, pk)
}

func TestMustSignDelegated_ReturnsSignedTx(t *testing.T) {
	delegatorPK, _ := crypto.GenerateKey()
	originPK, _ := crypto.GenerateKey()
	var features Features
	features.SetDelegated(true)
	trx := NewBuilder(TypeLegacy).Features(features).Build()
	signed := MustSignDelegated(trx, originPK, delegatorPK)
	assert.NotNil(t, signed)
}

func TestSign_ErrorFromCryptoSign(t *testing.T) {
	// go-ethereum's crypto.Sign panics on nil/invalid keys, so we expect a panic here.
	defer func() {
		if r := recover(); r == nil {
			t.Error("Sign should panic when given an invalid key (go-ethereum/crypto.Sign panics)")
		}
	}()
	pk := &ecdsa.PrivateKey{} // zero value, not valid
	trx := NewBuilder(TypeLegacy).Build()
	_, _ = Sign(trx, pk)
}

func TestSignDelegated_ErrorFromOriginSign(t *testing.T) {
	// go-ethereum's crypto.Sign panics on nil/invalid keys, so we expect a panic here.
	defer func() {
		if r := recover(); r == nil {
			t.Error("SignDelegated should panic when given an invalid origin key (go-ethereum/crypto.Sign panics)")
		}
	}()
	delegatorPK, _ := crypto.GenerateKey()
	originPK := &ecdsa.PrivateKey{} // invalid
	var features Features
	features.SetDelegated(true)
	trx := NewBuilder(TypeLegacy).Features(features).Build()
	_, _ = SignDelegated(trx, originPK, delegatorPK)
}

func TestSignDelegated_ErrorFromDelegatorSign(t *testing.T) {
	// go-ethereum's crypto.Sign panics on nil/invalid keys, so we expect a panic here.
	defer func() {
		if r := recover(); r == nil {
			t.Error("SignDelegated should panic when given an invalid delegator key (go-ethereum/crypto.Sign panics)")
		}
	}()
	delegatorPK := &ecdsa.PrivateKey{} // invalid
	originPK, _ := crypto.GenerateKey()
	var features Features
	features.SetDelegated(true)
	trx := NewBuilder(TypeLegacy).Features(features).Build()
	_, _ = SignDelegated(trx, originPK, delegatorPK)
}
