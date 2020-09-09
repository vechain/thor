// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	. "github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestBlock(t *testing.T) {
	var (
		proof     [81]byte
		signature [65]byte
	)
	rand.Read(proof[:])
	rand.Read(signature[:])
	bs, _ := NewComplexSignature(proof[:], signature[:])

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
	body := block.Body()
	bss := block.BackerSignatures()
	txsRootHash := txs.RootHash()
	brRootHash := bss.RootHash()

	fmt.Println(h.ID())

	assert.Equal(t, body.Txs, txs)
	assert.Equal(t, Compose(h, txs, ComplexSignatures(nil)), block)
	assert.Equal(t, gasLimit, h.GasLimit())
	assert.Equal(t, gasUsed, h.GasUsed())
	assert.Equal(t, totalScore, h.TotalScore())
	assert.Equal(t, emptyRoot, h.StateRoot())
	assert.Equal(t, emptyRoot, h.ReceiptsRoot())
	assert.Equal(t, now, h.Timestamp())
	assert.Equal(t, emptyRoot, h.ParentID())
	assert.Equal(t, beneficiary, h.Beneficiary())
	assert.Equal(t, txsRootHash, h.TxsRoot())
	assert.Equal(t, brRootHash, h.BackerSignaturesRoot())
	assert.Equal(t, uint64(0), h.TotalBackersCount())

	key, _ := crypto.HexToECDSA(privKey)
	sig, _ := crypto.Sign(block.Header().SigningHash().Bytes(), key)

	block = block.WithSignature(sig)

	data, _ := rlp.EncodeToBytes(block)

	fmt.Println(block.Size())

	b := Block{}
	rlp.DecodeBytes(data, &b)
	fmt.Println(b.Header().ID())
	fmt.Println(&b)

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
		BackerSignatures(ComplexSignatures{bs}, 10).
		Build()

	assert.Equal(t, tx.Features(1), block.Header().TxsFeatures())
	assert.Equal(t, uint64(10+1), block.Header().TotalBackersCount())
	assert.Equal(t, block.BackerSignatures().RootHash(), block.Header().BackerSignaturesRoot())

	data, _ = rlp.EncodeToBytes(block)
	var bx Block
	assert.Nil(t, rlp.DecodeBytes(data, &bx))
	assert.Equal(t, block.Header().ID(), bx.Header().ID())
	assert.Equal(t, block.Header().TxsFeatures(), bx.Header().TxsFeatures())
}

func TestEncoding(t *testing.T) {
	tx1 := new(tx.Builder).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).Build()
	tx2 := new(tx.Builder).Clause(tx.NewClause(nil)).Build()

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
	body := block.Body()
	bss := block.BackerSignatures()
	txsRootHash := txs.RootHash()
	brRootHash := bss.RootHash()

	assert.Equal(t, body.Txs, txs)
	assert.Equal(t, Compose(h, txs, ComplexSignatures(nil)), block)
	assert.Equal(t, gasLimit, h.GasLimit())
	assert.Equal(t, gasUsed, h.GasUsed())
	assert.Equal(t, totalScore, h.TotalScore())
	assert.Equal(t, emptyRoot, h.StateRoot())
	assert.Equal(t, emptyRoot, h.ReceiptsRoot())
	assert.Equal(t, now, h.Timestamp())
	assert.Equal(t, emptyRoot, h.ParentID())
	assert.Equal(t, beneficiary, h.Beneficiary())
	assert.Equal(t, txsRootHash, h.TxsRoot())
	assert.Equal(t, brRootHash, h.BackerSignaturesRoot())
	assert.Equal(t, uint64(0), h.TotalBackersCount())

	var raws []rlp.RawValue
	data, _ := rlp.EncodeToBytes(block)

	rlp.Decode(bytes.NewReader(data), &raws)
	assert.Equal(t, 2, len(raws))
}

func TestHeaderEncoding(t *testing.T) {
	block := new(Builder).Build()
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
}

func TestEncodingBadBssRoot(t *testing.T) {
	block := new(Builder).Build()
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
	assert.EqualValues(t, 0, h1.TotalBackersCount())

	data, _, err := rlp.SplitList(bytes)
	if err != nil {
		t.Fatal(err)
	}
	count, err := rlp.CountValues(data)
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, 10, count)

	var raws []rlp.RawValue
	_ = rlp.DecodeBytes(bytes, &raws)
	d, _ := rlp.EncodeToBytes(&struct {
		Root              thor.Bytes32
		TotalBackersCount uint64
	}{
		thor.Bytes32{},
		0,
	})
	raws = append(raws, d)
	b, _ := rlp.EncodeToBytes(raws)

	var h2 Header
	err = rlp.DecodeBytes(b, &h2)
	assert.EqualError(t, err, "rlp: BackerSignautreRoot should be trimmed if total backers count is 0")
}

func TestEncodingBssRoot(t *testing.T) {
	block := new(Builder).BackerSignatures(ComplexSignatures{}, 1).Build()
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
	assert.EqualValues(t, 1, hh.TotalBackersCount())

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

func TestDecoding(t *testing.T) {
	b0 := new(Builder).BackerSignatures(ComplexSignatures{}, 1).Build()
	b1 := new(Builder).Build()

	raw0, _ := rlp.EncodeToBytes(struct {
		A uint
	}{1})

	raw1, _ := rlp.EncodeToBytes([]interface{}{
		b0.Header(),
		b0.Transactions(),
		uint(1),
		uint(2),
	})

	raw2, _ := rlp.EncodeToBytes([]interface{}{
		b1.Header(),
		b1.Transactions(),
		uint(1),
	})

	var bx Block

	err := rlp.DecodeBytes(raw0, &bx)
	assert.EqualError(t, err, "rlp:invalid fields of block body, want 2 or 3")

	err = rlp.DecodeBytes(raw1, &bx)
	assert.EqualError(t, err, "rlp:invalid fields of block body, want 2 or 3")

	err = rlp.DecodeBytes(raw2, &bx)
	assert.EqualError(t, err, "rlp:unrecognized block format")
}
