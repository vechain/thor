// Copyright (c) 2024 The VeChainThor developers

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

	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func convertClause(c *tx.Clause) dto.Clause {
	return dto.Clause{
		To:    c.To(),
		Value: (*math.HexOrDecimal256)(c.Value()),
		Data:  hexutil.Encode(c.Data()),
	}
}

func TestConvertLegacyTransaction_Success(t *testing.T) {
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	cla2 := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	br := tx.NewBlockRef(0)
	transaction := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(123).
		GasPriceCoef(1).
		Expiration(10).
		Gas(37000).
		Nonce(1).
		Clause(cla).
		Clause(cla2).
		BlockRef(br).
		Build()

	header := new(block.Builder).Build().Header()

	result := ConvertTransaction(transaction, header)
	// Common fields
	clause := convertClause(cla)
	clause2 := convertClause(cla2)
	assert.Equal(t, transaction.Type(), result.Type)
	assert.Equal(t, hexutil.Encode(br[:]), result.BlockRef)
	assert.Equal(t, transaction.ChainTag(), result.ChainTag)
	assert.Equal(t, transaction.Expiration(), result.Expiration)
	assert.Equal(t, transaction.Gas(), result.Gas)
	assert.Equal(t, math.HexOrDecimal64(transaction.Nonce()), result.Nonce)
	assert.Equal(t, 2, len(result.Clauses))
	assert.Equal(t, addr, *result.Clauses[0].To)
	assert.Equal(t, &clause, result.Clauses[0])
	assert.Equal(t, addr, *result.Clauses[1].To)
	assert.Equal(t, &clause2, result.Clauses[1])
	// Legacy fields
	assert.Equal(t, uint8(1), *result.GasPriceCoef)
	// Non legacy fields
	assert.Empty(t, result.MaxFeePerGas)
	assert.Empty(t, result.MaxPriorityFeePerGas)
}

func TestConvertDynTransaction_Success(t *testing.T) {
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	cla2 := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	br := tx.NewBlockRef(0)
	maxFeePerGas := big.NewInt(25000)
	maxPriorityFeePerGas := big.NewInt(100)
	transaction := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(123).
		MaxFeePerGas(maxFeePerGas).
		MaxPriorityFeePerGas(maxPriorityFeePerGas).
		Expiration(10).
		Gas(37000).
		Nonce(1).
		Clause(cla).
		Clause(cla2).
		BlockRef(br).
		Build()

	header := new(block.Builder).Build().Header()

	result := ConvertTransaction(transaction, header)
	// Common fields
	clause := convertClause(cla)
	clause2 := convertClause(cla2)
	assert.Equal(t, transaction.Type(), result.Type)
	assert.Equal(t, hexutil.Encode(br[:]), result.BlockRef)
	assert.Equal(t, transaction.ChainTag(), result.ChainTag)
	assert.Equal(t, transaction.Expiration(), result.Expiration)
	assert.Equal(t, transaction.Gas(), result.Gas)
	assert.Equal(t, math.HexOrDecimal64(transaction.Nonce()), result.Nonce)
	assert.Equal(t, 2, len(result.Clauses))
	assert.Equal(t, addr, *result.Clauses[0].To)
	assert.Equal(t, &clause, result.Clauses[0])
	assert.Equal(t, addr, *result.Clauses[1].To)
	assert.Equal(t, &clause2, result.Clauses[1])
	// DynFee fields
	assert.Equal(t, (*math.HexOrDecimal256)(maxFeePerGas), result.MaxFeePerGas)
	assert.Equal(t, (*math.HexOrDecimal256)(maxPriorityFeePerGas), result.MaxPriorityFeePerGas)
	// Non dynFee fields
	assert.Empty(t, result.GasPriceCoef)
}

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
	addr := randAddress()

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
