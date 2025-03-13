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

func TestBuilder_Build(t *testing.T) {
	builder := &Builder{}

	id := thor.Bytes32{1, 2, 3}
	builder.ParentID(id)

	ts := uint64(1234567890)
	builder.Timestamp(ts)

	score := uint64(100)
	builder.TotalScore(score)

	limit := uint64(5000)
	builder.GasLimit(limit)

	used := uint64(3000)
	builder.GasUsed(used)

	addr := thor.Address{1, 2, 3}
	builder.Beneficiary(addr)

	hash := thor.Bytes32{1, 2, 3}
	builder.StateRoot(hash)

	builder.ReceiptsRoot(hash)
	tr := tx.NewBuilder(tx.TypeLegacy).Build()

	builder.Transaction(tr)
	features := tx.Features(0x01)

	builder.TransactionFeatures(features)
	alpha := []byte{1, 2, 3}

	builder.Alpha(alpha)
	builder.COM()

	baseFee := big.NewInt(1000)
	builder.BaseFee(baseFee)

	b := builder.Build()

	assert.Equal(t, id, b.header.ParentID())
	assert.Equal(t, ts, b.header.Timestamp())
	assert.Equal(t, score, b.header.TotalScore())
	assert.Equal(t, limit, b.header.GasLimit())
	assert.Equal(t, used, b.header.GasUsed())
	assert.Equal(t, addr, b.header.Beneficiary())
	assert.Equal(t, hash, b.header.StateRoot())
	assert.Equal(t, hash, b.header.ReceiptsRoot())
	assert.Contains(t, b.Transactions(), tr)
	assert.Equal(t, features, b.header.TxsFeatures())
	assert.Equal(t, alpha, b.header.Alpha())
	assert.True(t, b.header.COM())
	assert.Equal(t, baseFee, b.header.BaseFee())
}
