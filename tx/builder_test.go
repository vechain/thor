// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestNewTxBuilder(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	assert.NotNil(t, builder)
	assert.Equal(t, TypeLegacy, builder.txType)
}

func TestBuilder_ChainTag(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	builder.ChainTag(0x4a)
	assert.Equal(t, byte(0x4a), builder.chainTag)
}

func TestBuilder_Clause(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	addr := thor.BytesToAddress([]byte("to"))
	clause := NewClause(&addr)
	builder.Clause(clause)
	assert.Equal(t, 1, len(builder.clauses))
	assert.Equal(t, clause, builder.clauses[0])
}

func TestBuilder_GasPriceCoef(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	builder.GasPriceCoef(10)
	assert.Equal(t, uint8(10), builder.gasPriceCoef)
}

func TestBuilder_MaxFeePerGas(t *testing.T) {
	builder := NewTxBuilder(TypeDynamicFee)
	maxFee := big.NewInt(1000000000)
	builder.MaxFeePerGas(maxFee)
	assert.Equal(t, maxFee, builder.maxFeePerGas)
}

func TestBuilder_MaxPriorityFeePerGas(t *testing.T) {
	builder := NewTxBuilder(TypeDynamicFee)
	maxPriorityFee := big.NewInt(2000000000)
	builder.MaxPriorityFeePerGas(maxPriorityFee)
	assert.Equal(t, maxPriorityFee, builder.maxPriorityFeePerGas)
}

func TestBuilder_Gas(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	builder.Gas(21000)
	assert.Equal(t, uint64(21000), builder.gas)
}

func TestBuilder_BlockRef(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	blockRef := BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	builder.BlockRef(blockRef)
	assert.Equal(t, uint64(0x0102030405060708), builder.blockRef)
}

func TestBuilder_Expiration(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	builder.Expiration(100)
	assert.Equal(t, uint32(100), builder.expiration)
}

func TestBuilder_Nonce(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	builder.Nonce(12345)
	assert.Equal(t, uint64(12345), builder.nonce)
}

func TestBuilder_DependsOn(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	txID := thor.Bytes32{0x01, 0x02, 0x03, 0x04}
	builder.DependsOn(&txID)
	assert.Equal(t, &txID, builder.dependsOn)
	builder.DependsOn(nil)
	assert.Nil(t, builder.dependsOn)
}

func TestBuilder_Features(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy)
	features := Features(0x01)
	builder.Features(features)
	assert.Equal(t, features, builder.reserved.Features)
}

func TestBuilder_Build_Legacy(t *testing.T) {
	builder := NewTxBuilder(TypeLegacy).
		ChainTag(0x4a).
		Clause(&Clause{}).
		GasPriceCoef(10).
		Gas(21000).
		BlockRef(BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}).
		Expiration(100).
		Nonce(12345).
		DependsOn(&thor.Bytes32{0x01, 0x02, 0x03, 0x04}).
		Features(0x01)

	tx, err := builder.Build()
	assert.NoError(t, err)
	assert.NotNil(t, tx)
	assert.IsType(t, &LegacyTransaction{}, tx.body)
}

func TestBuilder_Build_DynamicFee(t *testing.T) {
	builder := NewTxBuilder(TypeDynamicFee).
		ChainTag(0x4a).
		Clause(&Clause{}).
		MaxFeePerGas(big.NewInt(1000000000)).
		MaxPriorityFeePerGas(big.NewInt(20000)).
		Gas(21000).
		BlockRef(BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}).
		Expiration(100).
		Nonce(12345).
		DependsOn(&thor.Bytes32{0x01, 0x02, 0x03, 0x04}).
		Features(0x01)

	tx, err := builder.Build()
	assert.NoError(t, err)
	assert.NotNil(t, tx)
	assert.IsType(t, &DynamicFeeTransaction{}, tx.body)
}

func TestBuilder_Build_InvalidType(t *testing.T) {
	builder := NewTxBuilder(0xff)
	tx, err := builder.Build()
	assert.Error(t, err)
	assert.Nil(t, tx)
	assert.Equal(t, ErrTxTypeNotSupported, err)
}
