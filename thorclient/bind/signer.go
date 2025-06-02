// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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

func NewSigner(privateKey *ecdsa.PrivateKey) *PrivateKeySigner {
	return (*PrivateKeySigner)(privateKey)
}

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
