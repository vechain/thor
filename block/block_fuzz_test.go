// Copyright (c) 2025 The VeChainThor developers

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
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

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

func FuzzHeaderEncoding(f *testing.F) {
	f.Fuzz(func(t *testing.T, addrBytes, beneficiary []byte, maxFee, gasUsed, gasLimit, totalScore uint64) {
		h0 := randomBlock(addrBytes, beneficiary, maxFee, gasUsed, gasLimit, totalScore).Header()
		enc, err := rlp.EncodeToBytes(h0)
		if err != nil {
			t.Errorf("failed to encode header: %v", err)
		}
		var decodedHeader Header
		if err := rlp.DecodeBytes(enc, &decodedHeader); err != nil {
			t.Errorf("failed to decode header: %v", err)
		}
		if h0.String() != decodedHeader.String() {
			t.Errorf("Header expected to be the same but: %v", err)
		}
	})
}

func FuzzBlockDecoding(f *testing.F) {
	f.Fuzz(func(t *testing.T, input []byte) {
		var b Block
		_ = rlp.DecodeBytes(input, &b)
	})
}
