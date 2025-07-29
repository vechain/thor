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

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	assert.NotNil(t, builder)
	assert.Equal(t, TypeLegacy, builder.txType)
}

func TestBuilder_ChainTag(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	builder.ChainTag(0x4a)
	assert.Equal(t, byte(0x4a), builder.chainTag)
}

func TestBuilder_Clause(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	addr := thor.BytesToAddress([]byte("to"))
	clause := NewClause(&addr)
	builder.Clause(clause)
	assert.Equal(t, 1, len(builder.clauses))
	assert.Equal(t, clause, builder.clauses[0])
}

func TestBuilder_GasPriceCoef(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	builder.GasPriceCoef(10)
	assert.Equal(t, uint8(10), builder.gasPriceCoef)
}

func TestBuilder_MaxFeePerGas(t *testing.T) {
	builder := NewBuilder(TypeDynamicFee)
	maxFee := big.NewInt(1000000000)
	builder.MaxFeePerGas(maxFee)
	assert.Equal(t, maxFee, builder.maxFeePerGas)
}

func TestBuilder_MaxPriorityFeePerGas(t *testing.T) {
	builder := NewBuilder(TypeDynamicFee)
	maxPriorityFee := big.NewInt(2000000000)
	builder.MaxPriorityFeePerGas(maxPriorityFee)
	assert.Equal(t, maxPriorityFee, builder.maxPriorityFeePerGas)
}

func TestBuilder_Gas(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	builder.Gas(21000)
	assert.Equal(t, uint64(21000), builder.gas)
}

func TestBuilder_BlockRef(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	blockRef := BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	builder.BlockRef(blockRef)
	assert.Equal(t, uint64(0x0102030405060708), builder.blockRef)
}

func TestBuilder_Expiration(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	builder.Expiration(100)
	assert.Equal(t, uint32(100), builder.expiration)
}

func TestBuilder_Nonce(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	builder.Nonce(12345)
	assert.Equal(t, uint64(12345), builder.nonce)
}

func TestBuilder_DependsOn(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	txID := thor.Bytes32{0x01, 0x02, 0x03, 0x04}
	builder.DependsOn(&txID)
	assert.Equal(t, &txID, builder.dependsOn)
	builder.DependsOn(nil)
	assert.Nil(t, builder.dependsOn)
}

func TestBuilder_Features(t *testing.T) {
	builder := NewBuilder(TypeLegacy)
	features := Features(0x01)
	builder.Features(features)
	assert.Equal(t, features, builder.reserved.Features)
}

func TestBuilder_Build_Legacy(t *testing.T) {
	builder := NewBuilder(TypeLegacy).
		ChainTag(0x4a).
		Clause(&Clause{}).
		GasPriceCoef(10).
		Gas(21000).
		BlockRef(BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}).
		Expiration(100).
		Nonce(12345).
		DependsOn(&thor.Bytes32{0x01, 0x02, 0x03, 0x04}).
		Features(0x01)

	tx := builder.Build()
	assert.NotNil(t, tx)
	assert.IsType(t, &legacyTransaction{}, tx.body)
}

func TestBuilder_Build_DynamicFee(t *testing.T) {
	builder := NewBuilder(TypeDynamicFee).
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

	tx := builder.Build()
	assert.NotNil(t, tx)
	assert.IsType(t, &dynamicFeeTransaction{}, tx.body)
}
