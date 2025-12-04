// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Test data generation utilities (copied from sqlite3 tests)

func newTx(txType tx.Type) *tx.Transaction {
	trx := tx.NewBuilder(txType).Build()

	pk, _ := crypto.GenerateKey()
	sig, _ := crypto.Sign(trx.Hash().Bytes(), pk)
	return trx.WithSignature(sig)
}

func newReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: datagen.RandAddress(),
					Topics:  []thor.Bytes32{datagen.RandomHash()},
					Data:    datagen.RandomHash().Bytes(),
				}},
				Transfers: tx.Transfers{{
					Sender:    datagen.RandAddress(),
					Recipient: datagen.RandAddress(),
					Amount:    new(big.Int).SetBytes(datagen.RandAddress().Bytes()),
				}},
			},
		},
	}
}
