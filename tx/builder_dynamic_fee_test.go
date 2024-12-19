// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
)

func TestDynFeeBuilder_ChainTag(t *testing.T) {
	builder := &DynFeeBuilder{}
	builder.ChainTag(0x4a)
	assert.Equal(t, byte(0x4a), builder.dynamicFeeTx.ChainTag)
}

func TestDynFeeBuilder_Clause(t *testing.T) {
	builder := &DynFeeBuilder{}
	addr := thor.BytesToAddress([]byte("to"))
	clause := NewClause(&addr)
	builder.Clause(clause)
	assert.Equal(t, 1, len(builder.dynamicFeeTx.Clauses))
	assert.Equal(t, clause, builder.dynamicFeeTx.Clauses[0])
}

func TestDynFeeBuilder_Gas(t *testing.T) {
	builder := &DynFeeBuilder{}
	builder.Gas(21000)
	assert.Equal(t, uint64(21000), builder.dynamicFeeTx.Gas)
}

func TestDynFeeBuilder_MaxFeePerGas(t *testing.T) {
	builder := &DynFeeBuilder{}
	maxFee := big.NewInt(1000000000)
	builder.MaxFeePerGas(maxFee)
	assert.Equal(t, maxFee, builder.dynamicFeeTx.MaxFeePerGas)
}

func TestDynFeeBuilder_MaxPriorityFeePerGas(t *testing.T) {
	builder := &DynFeeBuilder{}
	maxPriorityFee := big.NewInt(2000000000)
	builder.MaxPriorityFeePerGas(maxPriorityFee)
	assert.Equal(t, maxPriorityFee, builder.dynamicFeeTx.MaxPriorityFeePerGas)
}

func TestDynFeeBuilder_BlockRef(t *testing.T) {
	builder := &DynFeeBuilder{}
	blockRef := BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	builder.BlockRef(blockRef)
	assert.Equal(t, binary.BigEndian.Uint64(blockRef[:]), builder.dynamicFeeTx.BlockRef)
}

func TestDynFeeBuilder_Expiration(t *testing.T) {
	builder := &DynFeeBuilder{}
	builder.Expiration(720)
	assert.Equal(t, uint32(720), builder.dynamicFeeTx.Expiration)
}

func TestDynFeeBuilder_Nonce(t *testing.T) {
	builder := &DynFeeBuilder{}
	builder.Nonce(12345)
	assert.Equal(t, uint64(12345), builder.dynamicFeeTx.Nonce)
}

func TestDynFeeBuilder_DependsOn(t *testing.T) {
	builder := &DynFeeBuilder{}
	txID := thor.Bytes32{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	builder.DependsOn(&txID)
	assert.Equal(t, &txID, builder.dynamicFeeTx.DependsOn)
	builder.DependsOn(nil)
	assert.Nil(t, builder.dynamicFeeTx.DependsOn)
}

func TestDynFeeBuilder_Features(t *testing.T) {
	builder := &DynFeeBuilder{}
	features := Features(0x01)
	builder.Features(features)
	assert.Equal(t, features, builder.dynamicFeeTx.Reserved.Features)
}

func TestDynFeeBuilder_Build(t *testing.T) {
	builder := &DynFeeBuilder{}
	tx := builder.Build()
	assert.NotNil(t, tx)
	assert.Equal(t, &builder.dynamicFeeTx, tx.body)
}
