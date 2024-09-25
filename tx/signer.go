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

// MustSign signs a transaction with the provided private key using the default signing function.
// It panics if the signing process fails.
func MustSign(tx *Transaction, pk *ecdsa.PrivateKey) *Transaction {
	return MustSignFunc(crypto.Sign, tx, pk)
}

// MustSignFunc signs a transaction using a custom signing function and private key.
// It panics if the signing process fails.
func MustSignFunc(sign SignatureFunc, tx *Transaction, pk *ecdsa.PrivateKey) *Transaction {
	trx, err := SignFunc(sign, tx, pk)
	if err != nil {
		panic(err)
	}
	return trx
}

// Sign signs a transaction with the provided private key using the default signing function.
// It returns the signed transaction or an error if the signing fails.
func Sign(tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	return SignFunc(crypto.Sign, tx, pk)
}

// SignFunc signs a transaction using a custom signing function and private key.
// It returns the signed transaction or an error if the signing fails.
func SignFunc(sign SignatureFunc, tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	// Generate the signature for the transaction's signing hash.
	sig, err := sign(tx.SigningHash().Bytes(), pk)
	if err != nil {
		return nil, fmt.Errorf("unable to sign transaction: %w", err)
	}

	// Attach the signature to the transaction.
	return tx.WithSignature(sig), nil
}

// SignDelegator signs a transaction as a delegator with the provided private key using the default signing function.
// It returns the signed transaction or an error if the signing fails.
func SignDelegator(tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	return SignDelegatorFunc(crypto.Sign, tx, pk)
}

// SignDelegatorFunc signs a transaction as a delegator using a custom signing function and private key.
// It returns the signed transaction or an error if the signing fails.
func SignDelegatorFunc(sign SignatureFunc, tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	// Must have the delegated feature enabled
	if !tx.Features().IsDelegated() {
		return nil, errors.New("transaction delegated feature is not enabled")
	}

	// Must be signed by origin
	if len(tx.Signature()) != 65 {
		return nil, secp256k1.ErrInvalidSignatureLen
	}
	// Extract the public key from the existing transaction signature.
	pub, err := crypto.SigToPub(tx.SigningHash().Bytes(), tx.Signature())
	if err != nil {
		return nil, fmt.Errorf("unable to extract public key from signature: %w", err)
	}

	// Convert the public key to the corresponding address.
	origin := thor.Address(crypto.PubkeyToAddress(*pub))

	// Generate the delegator's signature using the delegator signing hash.
	dSig, err := sign(tx.DelegatorSigningHash(origin).Bytes(), pk)
	if err != nil {
		return nil, fmt.Errorf("unable to delegator sign transaction: %w", err)
	}

	// Append the delegator's signature to the existing transaction signature.
	sig := append(tx.Signature(), dSig...)

	// Attach the combined signature to the transaction.
	return tx.WithSignature(sig), nil
}
