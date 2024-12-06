// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

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
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestErrorWhileRetrievingTxOriginInConvertReceipt(t *testing.T) {
	tr := new(tx.LegacyBuilder).Build()
	header := &block.Header{}
	receipt := &tx.Receipt{
		Reward: big.NewInt(100),
		Paid:   big.NewInt(10),
	}

	convRec, err := convertReceipt(receipt, header, tr)

	assert.Error(t, err)
	assert.Equal(t, err, secp256k1.ErrInvalidSignatureLen)
	assert.Nil(t, convRec)
}

func TestConvertReceiptWhenTxHasNoClauseTo(t *testing.T) {
	value := big.NewInt(100)
	tr := newTx(tx.NewClause(nil).WithValue(value))
	b := new(block.Builder).Build()
	header := b.Header()
	receipt := newReceipt()
	expectedOutputAddress := thor.CreateContractAddress(tr.ID(), uint32(0), 0)

	convRec, err := convertReceipt(receipt, header, tr)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(convRec.Outputs))
	assert.Equal(t, &expectedOutputAddress, convRec.Outputs[0].ContractAddress)
}

func TestConvertReceipt(t *testing.T) {
	value := big.NewInt(100)
	addr := randAddress()
	tr := newTx(tx.NewClause(&addr).WithValue(value))
	b := new(block.Builder).Build()
	header := b.Header()
	receipt := newReceipt()

	convRec, err := convertReceipt(receipt, header, tr)

	assert.NoError(t, err)
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

// Utilities functions
func randAddress() (addr thor.Address) {
	rand.Read(addr[:])
	return
}

func newReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: randAddress(),
					Topics:  []thor.Bytes32{randomBytes32()},
					Data:    randomBytes32().Bytes(),
				}},
				Transfers: tx.Transfers{{
					Sender:    randAddress(),
					Recipient: randAddress(),
					Amount:    new(big.Int).SetBytes(randAddress().Bytes()),
				}},
			},
		},
		Reward: big.NewInt(100),
		Paid:   big.NewInt(10),
	}
}

func newTx(clause *tx.Clause) *tx.Transaction {
	tx := new(tx.LegacyBuilder).
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
