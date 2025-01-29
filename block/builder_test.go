// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestBuilder(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*testing.T)
	}{
		{
			name: "ParentID",
			fn: func(t *testing.T) {
				builder := &Builder{}
				id := thor.Bytes32{1, 2, 3}
				builder.ParentID(id)
				b := builder.Build()
				assert.Equal(t, id, b.header.ParentID())
			},
		},
		{
			name: "Timestamp",
			fn: func(t *testing.T) {
				builder := &Builder{}
				ts := uint64(1234567890)
				builder.Timestamp(ts)
				b := builder.Build()
				assert.Equal(t, ts, b.header.Timestamp())
			},
		},
		{
			name: "TotalScore",
			fn: func(t *testing.T) {
				builder := &Builder{}
				score := uint64(100)
				builder.TotalScore(score)
				b := builder.Build()
				assert.Equal(t, score, b.header.TotalScore())
			},
		},
		{
			name: "GasLimit",
			fn: func(t *testing.T) {
				builder := &Builder{}
				limit := uint64(5000)
				builder.GasLimit(limit)
				b := builder.Build()
				assert.Equal(t, limit, b.header.GasLimit())
			},
		},
		{
			name: "GasUsed",
			fn: func(t *testing.T) {
				builder := &Builder{}
				used := uint64(3000)
				builder.GasUsed(used)
				b := builder.Build()
				assert.Equal(t, used, b.header.GasUsed())
			},
		},
		{
			name: "Beneficiary",
			fn: func(t *testing.T) {
				builder := &Builder{}
				addr := thor.Address{1, 2, 3}
				builder.Beneficiary(addr)
				b := builder.Build()
				assert.Equal(t, addr, b.header.Beneficiary())
			},
		},
		{
			name: "StateRoot",
			fn: func(t *testing.T) {
				builder := &Builder{}
				hash := thor.Bytes32{1, 2, 3}
				builder.StateRoot(hash)
				b := builder.Build()
				assert.Equal(t, hash, b.header.StateRoot())
			},
		},
		{
			name: "ReceiptsRoot",
			fn: func(t *testing.T) {
				builder := &Builder{}
				hash := thor.Bytes32{1, 2, 3}
				builder.ReceiptsRoot(hash)
				b := builder.Build()
				assert.Equal(t, hash, b.header.ReceiptsRoot())
			},
		},
		{
			name: "Transaction",
			fn: func(t *testing.T) {
				builder := &Builder{}
				tx, _ := tx.NewTxBuilder(tx.LegacyTxType).Build()
				builder.Transaction(tx)
				b := builder.Build()
				assert.Contains(t, b.Transactions(), tx)
			},
		},
		{
			name: "TransactionFeatures",
			fn: func(t *testing.T) {
				builder := &Builder{}
				features := tx.Features(0x01)
				builder.TransactionFeatures(features)
				b := builder.Build()
				assert.Equal(t, features, b.header.TxsFeatures())
			},
		},
		{
			name: "Alpha",
			fn: func(t *testing.T) {
				builder := &Builder{}
				alpha := []byte{1, 2, 3}
				builder.Alpha(alpha)
				b := builder.Build()
				assert.Equal(t, alpha, b.header.Alpha())
			},
		},
		{
			name: "COM",
			fn: func(t *testing.T) {
				builder := &Builder{}
				builder.COM()
				b := builder.Build()
				assert.True(t, b.header.COM())
			},
		},
		{
			name: "BaseFee",
			fn: func(t *testing.T) {
				builder := &Builder{}
				baseFee := big.NewInt(1000)
				builder.BaseFee(baseFee)
				b := builder.Build()
				assert.Equal(t, baseFee, b.header.BaseFee())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t)
		})
	}
}
