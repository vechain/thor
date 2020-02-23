package block

import (
	"math"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vrf"
)

func randUint64() uint64 {
	return uint64(rand.Intn(math.MaxInt64))
}

func randBytes32() thor.Bytes32 {
	var b thor.Bytes32
	rand.Read(b[:])
	return b
}

func randProof() *vrf.Proof {
	var p vrf.Proof
	rand.Read(p[:])
	return &p
}

func randAddress() *thor.Address {
	var a thor.Address
	rand.Read(a[:])
	return &a
}

func randTx() *tx.Transaction {
	return new(tx.Builder).Clause(tx.NewClause(randAddress())).Build()
}

// RandTxs creates the designated number of random transactions
func RandTxs(N int) tx.Transactions {
	var txs tx.Transactions
	for i := 0; i < N; i++ {
		txs = append(txs, randTx())
	}
	return txs
}

// RandBlockSummary creates random block summary
func RandBlockSummary() *Summary {
	parentID := randBytes32()
	txsRoot := randBytes32()
	time := uint64(time.Now().Unix())
	score := randUint64()
	raw := NewBlockSummary(parentID, txsRoot, time, score)
	sk, _ := crypto.GenerateKey()
	sig, _ := crypto.Sign(raw.SigningHash().Bytes(), sk)
	raw = raw.WithSignature(sig)
	return raw
}

// RandEndorsement creates random endorsement
func RandEndorsement(raw *Summary) *Endorsement {
	ed := NewEndorsement(raw, randProof())
	sk, _ := crypto.GenerateKey()
	sig, _ := crypto.Sign(ed.SigningHash().Bytes(), sk)
	ed = ed.WithSignature(sig)
	return ed
}

// RandTxSet creates random tx set
func RandTxSet(n int) *TxSet {
	ts := NewTxSet(RandTxs(n), uint64(time.Now().Unix()), randUint64())
	sk, _ := crypto.GenerateKey()
	sig, _ := crypto.Sign(ts.SigningHash().Bytes(), sk)
	ts = ts.WithSignature(sig)
	return ts
}

// RandBlockHeader creates random block header
func RandBlockHeader() *Header {
	raw := RandBlockSummary()
	builder := new(Builder).
		ParentID(raw.ParentID()).
		Timestamp(raw.Timestamp()).
		TotalScore(raw.TotalScore()).
		SigOnBlockSummary(raw.Signature()).
		Beneficiary(*randAddress()).
		ReceiptsRoot(randBytes32()).
		StateRoot(randBytes32()).
		GasLimit(randUint64()).
		GasUsed(randUint64())

	for i := 0; i < int(thor.CommitteeSize); i++ {
		ed := RandEndorsement(raw)
		builder.VrfProof(ed.VrfProof()).Transaction(randTx())
	}

	return builder.Build().Header()
}
