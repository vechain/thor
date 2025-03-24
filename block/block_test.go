// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestBlock(t *testing.T) {
	tx1 := tx.NewBuilder(tx.TypeLegacy).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).Build()
	tx2 := tx.NewBuilder(tx.TypeDynamicFee).Clause(tx.NewClause(nil)).Build()

	privKey := string("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")

	now := uint64(time.Now().UnixNano())

	var (
		gasUsed     uint64       = 1000
		gasLimit    uint64       = 14000
		totalScore  uint64       = 101
		emptyRoot   thor.Bytes32 = thor.BytesToBytes32([]byte("0"))
		beneficiary thor.Address = thor.BytesToAddress([]byte("abc"))
	)

	blk := new(Builder).
		GasUsed(gasUsed).
		Transaction(tx1).
		Transaction(tx2).
		GasLimit(gasLimit).
		TotalScore(totalScore).
		StateRoot(emptyRoot).
		ReceiptsRoot(emptyRoot).
		Timestamp(now).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		ParentID(emptyRoot).
		Beneficiary(beneficiary).
		Build()

	h := blk.Header()

	txs := blk.Transactions()
	txsRootHash := txs.RootHash()

	assert.Equal(t, Compose(h, txs), blk)
	assert.Equal(t, gasLimit, h.GasLimit())
	assert.Equal(t, gasUsed, h.GasUsed())
	assert.Equal(t, totalScore, h.TotalScore())
	assert.Equal(t, emptyRoot, h.StateRoot())
	assert.Equal(t, emptyRoot, h.ReceiptsRoot())
	assert.Equal(t, now, h.Timestamp())
	assert.Equal(t, big.NewInt(thor.InitialBaseFee), h.BaseFee())
	assert.Equal(t, emptyRoot, h.ParentID())
	assert.Equal(t, beneficiary, h.Beneficiary())
	assert.Equal(t, txsRootHash, h.TxsRoot())

	key, _ := crypto.HexToECDSA(privKey)
	sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), key)

	blk = blk.WithSignature(sig)

	data, _ := rlp.EncodeToBytes(blk)

	b := Block{}
	rlp.DecodeBytes(data, &b)

	blk = new(Builder).
		GasUsed(gasUsed).
		GasLimit(gasLimit).
		TotalScore(totalScore).
		StateRoot(emptyRoot).
		ReceiptsRoot(emptyRoot).
		Timestamp(now).
		ParentID(emptyRoot).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		Beneficiary(beneficiary).
		TransactionFeatures(1).
		Build()

	sig, _ = crypto.Sign(blk.Header().SigningHash().Bytes(), key)
	blk = blk.WithSignature(sig)

	assert.Equal(t, tx.Features(1), blk.Header().TxsFeatures())
	data, _ = rlp.EncodeToBytes(blk)
	var bx Block
	assert.Nil(t, rlp.DecodeBytes(data, &bx))
	assert.Equal(t, blk.Header().ID(), bx.Header().ID())
	assert.Equal(t, blk.Header().TxsFeatures(), bx.Header().TxsFeatures())
}

func FuzzBlockEncoding(f *testing.F) {
	f.Fuzz(func(t *testing.T, arrdBytes, beneficiary []byte, maxFee, gasUsed, gasLimit, totalScore uint64) {
		newBlock := randomBlock(arrdBytes, beneficiary, maxFee, gasUsed, gasLimit, totalScore)
		enc, err := rlp.EncodeToBytes(newBlock)
		if err != nil {
			t.Errorf("failed to encode block: %v", err)
		}
		var decodedBlock Block
		if err := rlp.DecodeBytes(enc, &decodedBlock); err != nil {
			t.Errorf("failed to decode block: %v", err)
		}
		if err := checkBlockEquality(newBlock, &decodedBlock); err != nil {
			t.Errorf("Tx expected to be the same but: %v", err)
		}
	})
}

func randomBlock(addrBytes, byteArray []byte, maxFee, gasUsed, gasLimit, totalScore uint64) *Block {
	addr := thor.BytesToAddress(addrBytes)
	tx1 := tx.NewBuilder(tx.TypeLegacy).Clause(tx.NewClause(&addr)).Clause(tx.NewClause(&addr)).Build()
	tx2 := tx.NewBuilder(tx.TypeDynamicFee).MaxFeePerGas(big.NewInt(int64(maxFee))).Clause(tx.NewClause(&addr)).Clause(tx.NewClause(&addr)).Build()

	privKey := string("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")

	now := uint64(time.Now().UnixNano())

	var (
		root        thor.Bytes32 = thor.BytesToBytes32(byteArray)
		beneficiary thor.Address = thor.BytesToAddress(byteArray)
	)

	b := new(Builder).
		GasUsed(gasUsed).
		Transaction(tx1).
		Transaction(tx2).
		GasLimit(gasLimit).
		TotalScore(totalScore).
		StateRoot(root).
		ReceiptsRoot(root).
		Timestamp(now).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		ParentID(root).
		Beneficiary(beneficiary).
		Build()

	key, _ := crypto.HexToECDSA(privKey)
	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), key)

	return b.WithSignature(sig)
}

func checkBlockEquality(newBlock, decodedBlock *Block) error {
	if newBlock.Header().ID() != decodedBlock.Header().ID() {
		return fmt.Errorf("ID expected %v but got %v", newBlock.Header().ID(), decodedBlock.Header().ID())
	}
	if newBlock.Header().TxsRoot() != decodedBlock.Header().TxsRoot() {
		return fmt.Errorf("TxsRoot expected %v but got %v", newBlock.Header().TxsRoot(), decodedBlock.Header().TxsRoot())
	}
	if newBlock.Size() != decodedBlock.Size() {
		return fmt.Errorf("Size expected %v but got %v", newBlock.Size(), decodedBlock.Size())
	}
	if newBlock.Header().String() != decodedBlock.Header().String() {
		return fmt.Errorf("Header expected %v but got %v", newBlock.Header(), decodedBlock.Header())
	}
	for i, tx := range newBlock.Transactions() {
		if tx.ID() != decodedBlock.Transactions()[i].ID() {
			return fmt.Errorf("Tx expected %v but got %v", tx.String(), decodedBlock.Transactions()[i].String())
		}
	}

	return nil
}
