// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/test/datagen"
)

func FuzzTransactionMarshalling(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte, ui8 uint8, ui32 uint32, ui64 uint64) {
		txType := TypeLegacy
		if ui8%2 == 0 {
			txType = TypeDynamicFee
		}
		newTx := randomTx(b, ui8, ui32, ui64, txType)
		enc, err := newTx.MarshalBinary()
		if err != nil {
			t.Errorf("MarshalBinary: %v", err)
		}
		decTx := new(Transaction)
		err = decTx.UnmarshalBinary(enc)
		if err != nil {
			t.Errorf("UnmarshalBinary: %v", err)
		}
		if err := checkTxsEquality(newTx, decTx); err != nil {
			t.Errorf("Tx expected to be the same but: %v", err)
		}
	})
}

func randomTx(b []byte, ui8 uint8, ui32 uint32, ui64 uint64, txType Type) *Transaction {
	to := datagen.RandAddress()
	var b8 [8]byte
	copy(b8[:], b)
	i64 := int64(ui64)
	tag := datagen.RandBytes(1)[0]
	tr := NewBuilder(txType).ChainTag(tag).
		BlockRef(b8).
		Expiration(ui32).
		Clause(NewClause(&to).WithValue(big.NewInt(i64)).WithData(b)).
		Clause(NewClause(&to).WithValue(big.NewInt(i64)).WithData(b)).
		GasPriceCoef(ui8).
		MaxFeePerGas(big.NewInt(i64)).
		MaxPriorityFeePerGas(big.NewInt(i64)).
		Gas(ui64).
		DependsOn(nil).
		Nonce(ui64).Build()

	priv, _ := crypto.HexToECDSA("99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36")
	sig, _ := crypto.Sign(tr.SigningHash().Bytes(), priv)

	tr = tr.WithSignature(sig)
	return tr
}

func checkTxsEquality(expectedTx, actualTx *Transaction) error {
	if expectedTx.ID() != actualTx.ID() {
		return fmt.Errorf("ID: expected %v, got %v", expectedTx.ID(), actualTx.ID())
	}
	if expectedTx.Hash() != actualTx.Hash() {
		return fmt.Errorf("Hash: expected %v, got %v", expectedTx.Hash(), actualTx.Hash())
	}
	if expectedTx.SigningHash() != actualTx.SigningHash() {
		return fmt.Errorf("SigningHash: expected %v, got %v", expectedTx.SigningHash(), actualTx.SigningHash())
	}
	return nil
}

func FuzzTransactionDecoding(f *testing.F) {
	f.Fuzz(func(t *testing.T, input []byte) {
		var (
			trx1 Transaction
			trx2 Transaction
		)
		_ = rlp.DecodeBytes(input, &trx1)
		_ = trx2.UnmarshalBinary(input)
	})
}

func FuzzReceiptDecoding(f *testing.F) {
	f.Fuzz(func(t *testing.T, input []byte) {
		var (
			r1 Receipt
			r2 Receipt
		)
		_ = rlp.DecodeBytes(input, &r1)
		_ = r2.UnmarshalBinary(input)
	})
}
