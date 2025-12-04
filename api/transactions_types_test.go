// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestErrorWhileRetrievingTxOriginInConvertReceipt(t *testing.T) {
	txTypes := []tx.Type{tx.TypeLegacy, tx.TypeDynamicFee}

	for _, txType := range txTypes {
		tr := tx.NewBuilder(txType).Build()
		header := &block.Header{}
		receipt := &tx.Receipt{
			Reward: big.NewInt(100),
			Paid:   big.NewInt(10),
		}

		convRec, err := ConvertReceipt(receipt, header, tr)

		assert.Error(t, err)
		assert.Equal(t, err, secp256k1.ErrInvalidSignatureLen)
		assert.Nil(t, convRec)
	}
}

func TestConvertReceiptWhenTxHasNoClauseTo(t *testing.T) {
	value := big.NewInt(100)
	txs := []*tx.Transaction{
		newTx(tx.NewClause(nil).WithValue(value), tx.TypeLegacy),
		newTx(tx.NewClause(nil).WithValue(value), tx.TypeDynamicFee),
	}
	for _, tr := range txs {
		b := new(block.Builder).Build()
		header := b.Header()
		receipt := newReceipt()
		expectedOutputAddress := thor.CreateContractAddress(tr.ID(), uint32(0), 0)

		convRec, err := ConvertReceipt(receipt, header, tr)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(convRec.Outputs))
		assert.Equal(t, &expectedOutputAddress, convRec.Outputs[0].ContractAddress)
	}
}

func TestConvertReceipt(t *testing.T) {
	value := big.NewInt(100)
	addr := datagen.RandAddress()

	txs := []*tx.Transaction{
		newTx(tx.NewClause(&addr).WithValue(value), tx.TypeLegacy),
		newTx(tx.NewClause(&addr).WithValue(value), tx.TypeDynamicFee),
	}
	for _, tr := range txs {
		b := new(block.Builder).Build()
		header := b.Header()
		receipt := newReceipt()

		convRec, err := ConvertReceipt(receipt, header, tr)

		assert.NoError(t, err)
		assert.Equal(t, receipt.Type, convRec.Type)
		assert.Equal(t, 1, len(convRec.Outputs))
		assert.Equal(t, 1, len(convRec.Outputs[0].Events))
		assert.Equal(t, 1, len(convRec.Outputs[0].Transfers))
		assert.Nil(t, convRec.Outputs[0].ContractAddress)
		assert.Equal(t, receipt.Outputs[0].Events[0].Address, convRec.Outputs[0].Events[0].Address)
		assert.Equal(t, hexutil.Encode(receipt.Outputs[0].Events[0].Data), convRec.Outputs[0].Events[0].Data)
		assert.Equal(t, receipt.Outputs[0].Transfers[0].Sender, convRec.Outputs[0].Transfers[0].Sender)
		assert.Equal(t, receipt.Outputs[0].Transfers[0].Recipient, convRec.Outputs[0].Transfers[0].Recipient)
		assert.Equal(t, (*math.HexOrDecimal256)(receipt.Outputs[0].Transfers[0].Amount), convRec.Outputs[0].Transfers[0].Amount)
	}
}

// Utilities functions
func newReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: datagen.RandAddress(),
					Topics:  []thor.Bytes32{randomBytes32()},
					Data:    randomBytes32().Bytes(),
				}},
				Transfers: tx.Transfers{{
					Sender:    datagen.RandAddress(),
					Recipient: datagen.RandAddress(),
					Amount:    new(big.Int).SetBytes(datagen.RandAddress().Bytes()),
				}},
			},
		},
		Reward: big.NewInt(100),
		Paid:   big.NewInt(10),
	}
}

func newTx(clause *tx.Clause, txType tx.Type) *tx.Transaction {
	tx := tx.NewBuilder(txType).
		Clause(clause).
		Build()
	pk, _ := crypto.GenerateKey()
	sig, _ := crypto.Sign(tx.SigningHash().Bytes(), pk)
	return tx.WithSignature(sig)
}

func randomBytes32() thor.Bytes32 {
	var b32 thor.Bytes32

	rand.Read(b32[:])
	return b32
}
