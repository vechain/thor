// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block_test

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestBlock(t *testing.T) {
	tx1 := new(tx.Builder).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).BuildLegacy()
	tx2 := new(tx.Builder).Clause(tx.NewClause(nil)).BuildLegacy()

	privKey := string("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")

	now := uint64(time.Now().UnixNano())

	var (
		gasUsed     uint64       = 1000
		gasLimit    uint64       = 14000
		totalScore  uint64       = 101
		emptyRoot   thor.Bytes32 = thor.BytesToBytes32([]byte("0"))
		beneficiary thor.Address = thor.BytesToAddress([]byte("abc"))
	)

	blk := new(block.Builder).
		GasUsed(gasUsed).
		Transaction(tx1).
		Transaction(tx2).
		GasLimit(gasLimit).
		TotalScore(totalScore).
		StateRoot(emptyRoot).
		ReceiptsRoot(emptyRoot).
		Timestamp(now).
		ParentID(emptyRoot).
		Beneficiary(beneficiary).
		Build()

	h := blk.Header()

	txs := blk.Transactions()
	txsRootHash := txs.RootHash()

	assert.Equal(t, block.Compose(h, txs), blk)
	assert.Equal(t, gasLimit, h.GasLimit())
	assert.Equal(t, gasUsed, h.GasUsed())
	assert.Equal(t, totalScore, h.TotalScore())
	assert.Equal(t, emptyRoot, h.StateRoot())
	assert.Equal(t, emptyRoot, h.ReceiptsRoot())
	assert.Equal(t, now, h.Timestamp())
	assert.Equal(t, emptyRoot, h.ParentID())
	assert.Equal(t, beneficiary, h.Beneficiary())
	assert.Equal(t, txsRootHash, h.TxsRoot())

	key, _ := crypto.HexToECDSA(privKey)
	sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), key)

	blk = blk.WithSignature(sig)

	data, _ := rlp.EncodeToBytes(blk)

	b := block.Block{}
	rlp.DecodeBytes(data, &b)

	blk = new(block.Builder).
		GasUsed(gasUsed).
		GasLimit(gasLimit).
		TotalScore(totalScore).
		StateRoot(emptyRoot).
		ReceiptsRoot(emptyRoot).
		Timestamp(now).
		ParentID(emptyRoot).
		Beneficiary(beneficiary).
		TransactionFeatures(1).
		Build()

	sig, _ = crypto.Sign(blk.Header().SigningHash().Bytes(), key)
	blk = blk.WithSignature(sig)

	assert.Equal(t, tx.Features(1), blk.Header().TxsFeatures())
	data, _ = rlp.EncodeToBytes(blk)
	var bx block.Block
	assert.Nil(t, rlp.DecodeBytes(data, &bx))
	assert.Equal(t, blk.Header().ID(), bx.Header().ID())
	assert.Equal(t, blk.Header().TxsFeatures(), bx.Header().TxsFeatures())
}
