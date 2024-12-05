// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestLegacyBuilder_ChainTag(t *testing.T) {
	builder := &LegacyBuilder{}
	tag := byte(0x4a)
	builder.ChainTag(tag)

	assert.Equal(t, tag, builder.Build().ChainTag())
}

func TestLegacyBuilder_Clause(t *testing.T) {
	builder := &LegacyBuilder{}
	addr := thor.BytesToAddress([]byte("to"))
	clause := NewClause(&addr)
	builder.Clause(clause)

	assert.Equal(t, 1, len(builder.legacyTx.Clauses))
	assert.Equal(t, clause, builder.legacyTx.Clauses[0])
}

func TestLegacyBuilder_GasPriceCoef(t *testing.T) {
	builder := &LegacyBuilder{}
	coef := uint8(10)
	builder.GasPriceCoef(coef)

	assert.Equal(t, coef, builder.Build().GasPriceCoef())
}

func TestLegacyBuilder_Gas(t *testing.T) {
	builder := &LegacyBuilder{}
	gas := uint64(21000)
	builder.Gas(gas)

	assert.Equal(t, gas, builder.Build().Gas())
}

func TestLegacyBuilder_BlockRef(t *testing.T) {
	builder := &LegacyBuilder{}
	blockRef := BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	builder.BlockRef(blockRef)

	expected := binary.BigEndian.Uint32(blockRef[:])
	assert.Equal(t, expected, builder.Build().BlockRef().Number())
}

func TestLegacyBuilder_Expiration(t *testing.T) {
	builder := &LegacyBuilder{}
	expiration := uint32(100)
	builder.Expiration(expiration)

	assert.Equal(t, expiration, builder.Build().Expiration())
}

func TestLegacyBuilder_Nonce(t *testing.T) {
	builder := &LegacyBuilder{}
	nonce := uint64(12345)
	builder.Nonce(nonce)

	assert.Equal(t, nonce, builder.Build().Nonce())
}

func TestLegacyBuilder_DependsOn(t *testing.T) {
	builder := &LegacyBuilder{}
	txID := thor.Bytes32{0x01, 0x02, 0x03, 0x04}
	builder.DependsOn(&txID)

	assert.Equal(t, txID, *builder.Build().DependsOn())
}

func TestLegacyBuilder_Features(t *testing.T) {
	builder := &LegacyBuilder{}
	features := Features(0x01)
	builder.Features(features)

	assert.Equal(t, features, builder.Build().Features())
}
