// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/thor"
)

// SignatureFunc is a type alias for a function that signs a hash using a private key.
// It returns the signature and an error if any occurs.
type SignatureFunc func(hash []byte, prv *ecdsa.PrivateKey) (sig []byte, err error)

// MustSign signs a transaction using the provided private key and the default signing function.
// It panics if the signing process fails, returning a signed transaction upon success.
func MustSign(tx *Transaction, pk *ecdsa.PrivateKey) *Transaction {
	trx, err := signFunc(crypto.Sign, tx, pk)
	if err != nil {
		panic(err)
	}
	return trx
}

// Sign signs a transaction using the provided private key and the default signing function.
// It returns the signed transaction or an error if the signing process fails.
func Sign(tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	return signFunc(crypto.Sign, tx, pk)
}

// signFunc signs a transaction using a custom signing function and the provided private key.
// It returns the signed transaction or an error if the signing process fails.
func signFunc(sign SignatureFunc, tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	// Generate the signature for the transaction's signing hash.
	sig, err := sign(tx.SigningHash().Bytes(), pk)
	if err != nil {
		return nil, fmt.Errorf("unable to sign transaction: %w", err)
	}

	// Attach the signature to the transaction and return the signed transaction.
	return tx.WithSignature(sig), nil
}

// MustSignDelegator signs a transaction as a delegator using the provided private keys and the default signing function.
// It panics if the signing process fails, returning a signed transaction upon success.
func MustSignDelegator(tx *Transaction, originPK *ecdsa.PrivateKey, delegatorPK *ecdsa.PrivateKey) *Transaction {
	trx, err := signDelegatorFunc(crypto.Sign, tx, originPK, delegatorPK)
	if err != nil {
		panic(err)
	}
	return trx
}

// SignDelegator signs a transaction as a delegator using the provided private keys and the default signing function.
// It returns the signed transaction or an error if the signing process fails.
func SignDelegator(tx *Transaction, originPK *ecdsa.PrivateKey, delegatorPK *ecdsa.PrivateKey) (*Transaction, error) {
	return signDelegatorFunc(crypto.Sign, tx, originPK, delegatorPK)
}

// signDelegatorFunc signs a transaction as a delegator using a custom signing function and the provided private keys.
// It returns the signed transaction or an error if the signing process fails.
func signDelegatorFunc(sign SignatureFunc, unsignedTx *Transaction, originPK *ecdsa.PrivateKey, delegatorPK *ecdsa.PrivateKey) (*Transaction, error) {
	// Ensure the transaction has the delegated feature enabled.
	if !unsignedTx.Features().IsDelegated() {
		return nil, errors.New("transaction delegated feature is not enabled")
	}

	// Ensure the transaction has not already been signed by the origin.
	if len(unsignedTx.Signature()) != 0 {
		return nil, secp256k1.ErrInvalidSignatureLen
	}

	// Sign the transaction using the origin's private key.
	signedTx, err := Sign(unsignedTx, originPK)
	if err != nil {
		return nil, err
	}

	// Convert the origin's public key to its corresponding address.
	origin := thor.Address(crypto.PubkeyToAddress(originPK.PublicKey))

	// Generate the delegator's signature using the transaction's delegator signing hash.
	dSig, err := sign(signedTx.DelegatorSigningHash(origin).Bytes(), delegatorPK)
	if err != nil {
		return nil, fmt.Errorf("unable to delegator sign transaction: %w", err)
	}

	// Append the delegator's signature to the origin's signature.
	sig := append(signedTx.Signature(), dSig...)

	// Attach the combined signature to the transaction and return the signed transaction.
	return signedTx.WithSignature(sig), nil
}
