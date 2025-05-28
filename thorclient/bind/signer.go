package bind

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type Signer interface {
	Address() thor.Address
	SignTransaction(tx *tx.Transaction) (*tx.Transaction, error)
}
type PrivateKeySigner ecdsa.PrivateKey

var _ Signer = (*PrivateKeySigner)(nil)

func (p *PrivateKeySigner) Address() thor.Address {
	return thor.Address(crypto.PubkeyToAddress(p.PublicKey))
}

func (p *PrivateKeySigner) SignTransaction(trx *tx.Transaction) (*tx.Transaction, error) {
	signedTx, err := tx.Sign(trx, (*ecdsa.PrivateKey)(p))
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}
	return signedTx, nil
}
