// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block_test

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	. "github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vrf"
)

func getRndBytes32() thor.Bytes32 {
	var b thor.Bytes32
	rand.Read(b[:])
	return b
}

func TestBlock(t *testing.T) {

	tx1 := new(tx.Builder).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).Build()
	tx2 := new(tx.Builder).Clause(tx.NewClause(nil)).Build()

	privKey := string("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")

	now := uint64(time.Now().UnixNano())

	var (
		gasUsed     uint64       = 1000
		gasLimit    uint64       = 14000
		totalScore  uint64       = 101
		emptyRoot   thor.Bytes32 = thor.BytesToBytes32([]byte("0"))
		beneficiary thor.Address = thor.BytesToAddress([]byte("abc"))
		parentID    thor.Bytes32 = getRndBytes32()
		stateRoot   thor.Bytes32 = getRndBytes32()
		receiptRoot thor.Bytes32 = getRndBytes32()
		txsRoot     thor.Bytes32 = tx.Transactions{tx1, tx2}.RootHash()
	)

	var pfs []*vrf.Proof
	for i := 0; i < 5; i++ {
		pf := &vrf.Proof{}
		rand.Read(pf[:])
		pfs = append(pfs, pf)
	}

	bs := NewBlockSummary(parentID, txsRoot, totalScore, now)
	key, _ := crypto.HexToECDSA(privKey)
	sig, _ := crypto.Sign(bs.SigningHash().Bytes(), key)

	var sigs [][]byte
	for i := 0; i < 5; i++ {
		ed := NewEndorsement(bs, pfs[i])
		sig, _ := crypto.Sign(ed.SigningHash().Bytes(), key)
		sigs = append(sigs, sig)
	}

	block := new(Builder).
		GasUsed(gasUsed).
		Transaction(tx1).
		Transaction(tx2).
		GasLimit(gasLimit).
		TotalScore(totalScore).
		StateRoot(stateRoot).
		ReceiptsRoot(receiptRoot).
		Timestamp(now).
		ParentID(parentID).
		Beneficiary(beneficiary).
		SigOnBlockSummary(sig).SigsOnEndorsement(sigs).VrfProofs(pfs).
		Build()

	h := block.Header()

	txs := block.Transactions()
	body := block.Body()
	txsRootHash := txs.RootHash()

	fmt.Println(h.ID())

	assert.Equal(t, body.Txs, txs)
	assert.Equal(t, Compose(h, txs), block)
	assert.Equal(t, gasLimit, h.GasLimit())
	assert.Equal(t, gasUsed, h.GasUsed())
	assert.Equal(t, totalScore, h.TotalScore())
	assert.Equal(t, stateRoot, h.StateRoot())
	assert.Equal(t, receiptRoot, h.ReceiptsRoot())
	assert.Equal(t, now, h.Timestamp())
	assert.Equal(t, parentID, h.ParentID())
	assert.Equal(t, beneficiary, h.Beneficiary())
	assert.Equal(t, txsRootHash, h.TxsRoot())

	assert.Equal(t, sig, h.SigOnBlockSummary())
	for i := range pfs {
		assert.Equal(t, pfs[i], h.VrfProofs()[i])
		assert.Equal(t, sigs[i], h.SigsOnEndoresment()[i])
	}

	key, _ = crypto.HexToECDSA(privKey)
	sig, _ = crypto.Sign(block.Header().SigningHash().Bytes(), key)

	block = block.WithSignature(sig)
	h = block.Header()

	data, _ := rlp.EncodeToBytes(block)
	// fmt.Println(Raw(data).DecodeHeader())
	// fmt.Println(Raw(data).DecodeBody())
	dh, _ := Raw(data).DecodeHeader()
	assert.Equal(t, dh.GasLimit(), h.GasLimit())
	assert.Equal(t, dh.GasUsed(), h.GasUsed())
	assert.Equal(t, dh.TotalScore(), h.TotalScore())
	assert.Equal(t, dh.StateRoot(), h.StateRoot())
	assert.Equal(t, dh.ReceiptsRoot(), h.ReceiptsRoot())
	assert.Equal(t, dh.Timestamp(), h.Timestamp())
	assert.Equal(t, dh.ParentID(), h.ParentID())
	assert.Equal(t, dh.Beneficiary(), h.Beneficiary())
	assert.Equal(t, dh.TxsRoot(), h.TxsRoot())
	assert.Equal(t, dh.Signature(), h.Signature())

	assert.Equal(t, dh.SigOnBlockSummary(), h.SigOnBlockSummary())
	for i := range h.VrfProofs() {
		assert.Equal(t, dh.VrfProofs()[i], h.VrfProofs()[i])
		assert.Equal(t, dh.SigsOnEndoresment()[i], h.SigsOnEndoresment()[i])
	}

	body, _ = Raw(data).DecodeBody()
	assert.Equal(t, body.Txs.RootHash(), block.Transactions().RootHash())

	fmt.Println(block.Size())

	b := Block{}
	rlp.DecodeBytes(data, &b)
	// fmt.Println(b.Header().ID())
	// fmt.Println(&b)

	block = new(Builder).
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

	assert.Equal(t, tx.Features(1), block.Header().TxsFeatures())
	data, _ = rlp.EncodeToBytes(block)
	var bx Block
	assert.Nil(t, rlp.DecodeBytes(data, &bx))
	assert.Equal(t, block.Header().ID(), bx.Header().ID())
	assert.Equal(t, block.Header().TxsFeatures(), bx.Header().TxsFeatures())
}
