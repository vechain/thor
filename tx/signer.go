package tx

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/v2/thor"
)

// SignFunction is a type alias for a function that signs a hash using a private key.
// It returns the signature and an error if any occurs.
type SignFunction func(hash []byte, prv *ecdsa.PrivateKey) (sig []byte, err error)

// MustSignTx signs a transaction with the provided private key using the default signing function.
// It panics if the signing process fails.
func MustSignTx(tx *Transaction, pk *ecdsa.PrivateKey) *Transaction {
	return MustCustomSignTx(crypto.Sign, tx, pk)
}

// MustCustomSignTx signs a transaction using a custom signing function and private key.
// It panics if the signing process fails.
func MustCustomSignTx(sign SignFunction, tx *Transaction, pk *ecdsa.PrivateKey) *Transaction {
	trx, err := CustomSignTx(sign, tx, pk)
	if err != nil {
		panic(err)
	}
	return trx
}

// SignTx signs a transaction with the provided private key using the default signing function.
// It returns the signed transaction or an error if the signing fails.
func SignTx(tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	return CustomSignTx(crypto.Sign, tx, pk)
}

// CustomSignTx signs a transaction using a custom signing function and private key.
// It returns the signed transaction or an error if the signing fails.
func CustomSignTx(sign SignFunction, tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	// Generate the signature for the transaction's signing hash.
	sig, err := sign(tx.SigningHash().Bytes(), pk)
	if err != nil {
		return nil, fmt.Errorf("unable to sign transaction: %w", err)
	}

	// Attach the signature to the transaction.
	return tx.WithSignature(sig), nil
}

// DelegatorSignTx signs a transaction as a delegator with the provided private key using the default signing function.
// It returns the signed transaction or an error if the signing fails.
func DelegatorSignTx(tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
	return CustomDelegatorSignTx(crypto.Sign, tx, pk)
}

// CustomDelegatorSignTx signs a transaction as a delegator using a custom signing function and private key.
// It returns the signed transaction or an error if the signing fails.
func CustomDelegatorSignTx(sign SignFunction, tx *Transaction, pk *ecdsa.PrivateKey) (*Transaction, error) {
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
