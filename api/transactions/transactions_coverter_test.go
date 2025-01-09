// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestConvertLegacyTransaction_Success(t *testing.T) {
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	cla2 := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	br := tx.NewBlockRef(0)
	transaction, _ := tx.NewTxBuilder(tx.LegacyTxType).
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

	result := convertTransaction(transaction, header)
	// Common fields
	assert.Equal(t, hexutil.Encode(br[:]), result.BlockRef)
	assert.Equal(t, transaction.ChainTag(), result.ChainTag)
	assert.Equal(t, transaction.Expiration(), result.Expiration)
	assert.Equal(t, transaction.Gas(), result.Gas)
	assert.Equal(t, math.HexOrDecimal64(transaction.Nonce()), result.Nonce)
	assert.Equal(t, 2, len(result.Clauses))
	assert.Equal(t, addr, *result.Clauses[0].To)
	assert.Equal(t, convertClause(cla), result.Clauses[0])
	assert.Equal(t, addr, *result.Clauses[1].To)
	assert.Equal(t, convertClause(cla2), result.Clauses[1])
	// Legacy fields
	assert.Equal(t, uint8(1), result.GasPriceCoef)
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
	transaction, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).
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

	result := convertTransaction(transaction, header)
	// Common fields
	assert.Equal(t, hexutil.Encode(br[:]), result.BlockRef)
	assert.Equal(t, transaction.ChainTag(), result.ChainTag)
	assert.Equal(t, transaction.Expiration(), result.Expiration)
	assert.Equal(t, transaction.Gas(), result.Gas)
	assert.Equal(t, math.HexOrDecimal64(transaction.Nonce()), result.Nonce)
	assert.Equal(t, 2, len(result.Clauses))
	assert.Equal(t, addr, *result.Clauses[0].To)
	assert.Equal(t, convertClause(cla), result.Clauses[0])
	assert.Equal(t, addr, *result.Clauses[1].To)
	assert.Equal(t, convertClause(cla2), result.Clauses[1])
	// DynFee fields
	assert.Equal(t, maxFeePerGas, result.MaxFeePerGas)
	assert.Equal(t, maxPriorityFeePerGas, result.MaxPriorityFeePerGas)
	// Non dynFee fields
	assert.Empty(t, result.GasPriceCoef)
}
