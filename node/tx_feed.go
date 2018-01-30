package node

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/tx"
)

type fakeTxFeed struct {
	i int
}

var nonce = uint64(time.Now().UnixNano())

func (tf *fakeTxFeed) Next() *tx.Transaction {
	if tf.i < 100 {
		accs := genesis.Dev.Accounts()
		a0 := accs[0]
		a1 := accs[1]

		tx := new(tx.Builder).Clause(contracts.Energy.PackTransfer(a1.Address, big.NewInt(1))).
			Gas(300000).GasPrice(big.NewInt(2)).Nonce(nonce).Build()
		nonce++
		sig, _ := crypto.Sign(tx.SigningHash().Bytes(), a0.PrivateKey)
		tx = tx.WithSignature(sig)

		tf.i++
		return tx
	}

	return nil
}

func (tf *fakeTxFeed) MarkTxBad(tx *tx.Transaction) {

}
