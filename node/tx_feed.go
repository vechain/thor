package node

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type fakeTxFeed struct {
	i int
}

var nonce uint64 = uint64(time.Now().UnixNano())

func (ti *fakeTxFeed) HasNext() bool {
	return ti.i < 100
}

func (ti *fakeTxFeed) Next() *tx.Transaction {
	ti.i++

	accs := genesis.Dev.Accounts()
	a0 := accs[0]
	a1 := accs[1]

	tx := new(tx.Builder).
		ChainTag(2).
		Clause(contracts.Energy.PackTransfer(a1.Address, big.NewInt(1))).
		Gas(300000).Nonce(nonce).Build()
	nonce++
	sig, _ := crypto.Sign(tx.SigningHash().Bytes(), a0.PrivateKey)
	tx = tx.WithSignature(sig)

	return tx
}

func (ti *fakeTxFeed) OnProcessed(txID thor.Hash, err error) {
}
