// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	. "github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

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
	)

	block := new(Builder).
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

	h := block.Header()

	txs := block.Transactions()
	txsRootHash := txs.RootHash()

	assert.Equal(t, Compose(h, txs), block)
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
	sig, _ := crypto.Sign(block.Header().SigningHash().Bytes(), key)

	block = block.WithSignature(sig)

	data, _ := rlp.EncodeToBytes(block)

	b := Block{}
	rlp.DecodeBytes(data, &b)

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

	sig, _ = crypto.Sign(block.Header().SigningHash().Bytes(), key)
	block = block.WithSignature(sig)

	assert.Equal(t, tx.Features(1), block.Header().TxsFeatures())
	data, _ = rlp.EncodeToBytes(block)
	var bx Block
	assert.Nil(t, rlp.DecodeBytes(data, &bx))
	assert.Equal(t, block.Header().ID(), bx.Header().ID())
	assert.Equal(t, block.Header().TxsFeatures(), bx.Header().TxsFeatures())
}

func TestHeaderEncoding(t *testing.T) {
	var sig [65]byte
	rand.Read(sig[:])

	block := new(Builder).Build().WithSignature(sig[:])
	h := block.Header()

	bytes, err := rlp.EncodeToBytes(h)
	if err != nil {
		t.Fatal(err)
	}

	var hh Header
	err = rlp.DecodeBytes(bytes, &hh)
	if err != nil {
		t.Fatal(err)
	}

	bytes = append(bytes, []byte("just trailing")...)
	var hhh Header
	err = rlp.DecodeBytes(bytes, &hhh)
	assert.EqualError(t, err, "rlp: input contains more than one value")

	var proof [81]byte
	var alpha [32]byte
	rand.Read(proof[:])
	rand.Read(alpha[:])

	complex, err := NewComplexSignature(sig[:], proof[:])
	if err != nil {
		t.Fatal(err)
	}

	b1 := new(Builder).Alpha(alpha[:]).Build().WithSignature(complex[:])
	bs1, err := rlp.EncodeToBytes(b1.Header())
	if err != nil {
		t.Fatal(err)
	}

	var h1 Header
	err = rlp.DecodeBytes(bs1, &h1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEncodingBadExtension(t *testing.T) {
	var sig [65]byte
	rand.Read(sig[:])

	block := new(Builder).Build().WithSignature(sig[:])
	h := block.Header()

	bytes, err := rlp.EncodeToBytes(h)
	if err != nil {
		t.Fatal(err)
	}

	var h1 Header
	err = rlp.DecodeBytes(bytes, &h1)
	if err != nil {
		t.Fatal(err)
	}

	data, _, err := rlp.SplitList(bytes)
	if err != nil {
		t.Fatal(err)
	}
	count, err := rlp.CountValues(data)
	if err != nil {
		t.Fatal(err)
	}
	// backward compatiability，required to be trimmed
	assert.EqualValues(t, 10, count)

	var raws []rlp.RawValue
	_ = rlp.DecodeBytes(bytes, &raws)
	d, _ := rlp.EncodeToBytes(&struct {
		Alpha []byte
	}{
		[]byte{},
	})
	raws = append(raws, d)
	b, _ := rlp.EncodeToBytes(raws)

	var h2 Header
	err = rlp.DecodeBytes(b, &h2)
	assert.EqualError(t, err, "rlp: extension must be trimmed")
}

func TestEncodingExtension(t *testing.T) {
	var sig [block.ComplexSigSize]byte
	var alpha [32]byte
	rand.Read(sig[:])
	rand.Read(alpha[:])

	block := new(Builder).Alpha(alpha[:]).Build().WithSignature(sig[:])
	h := block.Header()

	bytes, err := rlp.EncodeToBytes(h)
	if err != nil {
		t.Fatal(err)
	}

	var hh Header
	err = rlp.DecodeBytes(bytes, &hh)
	if err != nil {
		t.Fatal(err)
	}

	data, _, err := rlp.SplitList(bytes)
	if err != nil {
		t.Fatal(err)
	}
	count, err := rlp.CountValues(data)
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, 11, count)
}

func TestCodingCompatibility(t *testing.T) {
	raw := hexutil.MustDecode("0xf8e0a0000000000000000000000000000000000000000000000000000000000000000080809400000000000000000000000000000000000000008080a045b0cfc220ceec5b7c1c62c4d4193d38e4eba48e8815729ce75f9c0ab0e4c1c0a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000b841e95a07bda136baa1181f32fba25b8dec156dee373781fdc7d24acd5e60ebc104c04b397ee7a67953e2d10acc4835343cd949a73e7e58db1b92f682db62e793c412")

	var h0 Header
	err := rlp.DecodeBytes(raw, &h0)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := rlp.EncodeToBytes(&h0)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, raw, bytes)

	data, _, err := rlp.SplitList(bytes)
	if err != nil {
		t.Fatal(err)
	}
	count, err := rlp.CountValues(data)
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, 10, count)
}
