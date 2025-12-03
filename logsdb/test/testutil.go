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

func newEventOnlyReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: datagen.RandAddress(),
					Topics:  []thor.Bytes32{datagen.RandomHash()},
					Data:    datagen.RandomHash().Bytes(),
				}},
			},
		},
	}
}

// createRichReceipt creates a receipt with multiple events and transfers for testing
func createRichReceipt(eventCount, transferCount int) *tx.Receipt {
	outputs := make([]*tx.Output, 1)

	// Create events
	events := make(tx.Events, eventCount)
	for i := 0; i < eventCount; i++ {
		events[i] = &tx.Event{
			Address: datagen.RandAddress(),
			Topics:  []thor.Bytes32{datagen.RandomHash(), datagen.RandomHash()},
			Data:    datagen.RandomHash().Bytes(),
		}
	}

	// Create transfers
	transfers := make(tx.Transfers, transferCount)
	for i := 0; i < transferCount; i++ {
		transfers[i] = &tx.Transfer{
			Sender:    datagen.RandAddress(),
			Recipient: datagen.RandAddress(),
			Amount:    new(big.Int).SetBytes(datagen.RandomHash().Bytes()),
		}
	}

	outputs[0] = &tx.Output{
		Events:    events,
		Transfers: transfers,
	}

	return &tx.Receipt{Outputs: outputs}
}

// createVTHOReceipt creates a receipt that looks like a VTHO transfer
func createVTHOReceipt() *tx.Receipt {
	vthoAddr := thor.MustParseAddress("0x0000000000000000000000000000456E65726779")
	vthoTopic := thor.MustParseBytes32("0xDDF252AD1BE2C89B69C2B068FC378DAA952BA7F163C4A11628F55A4DF523B3EF")

	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: vthoAddr,
					Topics:  []thor.Bytes32{vthoTopic, datagen.RandomHash(), datagen.RandomHash()},
					Data:    datagen.RandomHash().Bytes(),
				}},
				Transfers: tx.Transfers{{
					Sender:    datagen.RandAddress(),
					Recipient: datagen.RandAddress(),
					Amount:    new(big.Int).SetBytes(datagen.RandomHash().Bytes()),
				}},
			},
		},
	}
}
