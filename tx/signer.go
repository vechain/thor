// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/thor"
)

// MustSign signs a transaction using the provided private key and the default signing function.
// It panics if the signing process fails, returning a signed transaction upon success.
func MustSign(tx *Transaction, pk *ecdsa.PrivateKey) *Transaction {
	trx, err := Sign(tx, pk)
	if err != nil {
		panic(err)
	}
	return trx
}

// Sign signs a transaction using the provided private key and the default signing function.
// It returns the signed transaction or an error if the signing process fails.
func Sign(tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	// Generate the signature for the transaction's signing hash.
	sig, err := crypto.Sign(tx.SigningHash().Bytes(), pk)
	if err != nil {
		return nil, fmt.Errorf("unable to sign transaction: %w", err)
	}

	// Attach the signature to the transaction and return the signed transaction.
	return tx.WithSignature(sig), nil
}

// MustSignDelegated signs a transaction as a delegator using the provided private keys and the default signing function.
// It panics if the signing process fails, returning a signed transaction upon success.
func MustSignDelegated(tx *Transaction, originPK *ecdsa.PrivateKey, delegatorPK *ecdsa.PrivateKey) *Transaction {
	trx, err := SignDelegated(tx, originPK, delegatorPK)
	if err != nil {
		panic(err)
	}
	return trx
}

// SignDelegated signs a transaction as a delegator using the provided private keys and the default signing function.
// It returns the signed transaction or an error if the signing process fails.
func SignDelegated(tx *Transaction, originPK *ecdsa.PrivateKey, delegatorPK *ecdsa.PrivateKey) (*Transaction, error) {
	// Ensure the transaction has the delegated feature enabled.
	if !tx.Features().IsDelegated() {
		return nil, errors.New("transaction delegated feature is not enabled")
	}

	// Sign the transaction using the origin's private key.
	// Generate the signature for the transaction's signing hash.
	originSig, err := crypto.Sign(tx.SigningHash().Bytes(), originPK)
	if err != nil {
		return nil, fmt.Errorf("unable to sign transaction: %w", err)
	}

	// Convert the origin's public key to its corresponding address.
	origin := thor.Address(crypto.PubkeyToAddress(originPK.PublicKey))

	// Generate the delegator's signature using the transaction's delegator signing hash.
	dSig, err := crypto.Sign(tx.DelegatorSigningHash(origin).Bytes(), delegatorPK)
	if err != nil {
		return nil, fmt.Errorf("unable to delegator sign transaction: %w", err)
	}

	// Append the delegator's signature to the origin's signature.
	sig := append(originSig, dSig...)

	// Attach the combined signature to the transaction and return the signed transaction.
	return tx.WithSignature(sig), nil
}
