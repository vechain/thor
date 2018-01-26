package node

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/fortest"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/tx"
)

type fakeTxFeed struct {
	i int
}

var nonce = uint64(time.Now().UnixNano())

func (tf *fakeTxFeed) Next() *tx.Transaction {
	if tf.i < 100 {
		a0 := fortest.Accounts[0]
		a1 := fortest.Accounts[1]

		tx := new(tx.Builder).Clause(contracts.Energy.PackTransfer(a1.Address, big.NewInt(1))).
			Gas(300000).Nonce(nonce).Build()
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

func packerService(ctx context.Context, bp *blockPool, chain *chain.Chain, pk *packer.Packer, privateKey *ecdsa.PrivateKey) {
	for {
		best, err := chain.GetBestBlock()
		if err != nil {
			log.Fatalln(err)
		}

		now := uint64(time.Now().Unix())
		ts, pack, err := pk.Prepare(best.Header(), now)
		if err != nil {
			log.Fatalln(err)
		}

		timeout := time.After(time.Duration(ts-now) * time.Second)

		select {
		case <-ctx.Done():
			fmt.Println("proposerService exit")
			return
		case <-timeout:
			block, _, err := pack(&fakeTxFeed{})
			if err != nil {
				log.Fatalln(err)
			}

			sig, err := crypto.Sign(block.Header().SigningHash().Bytes(), privateKey)
			if err != nil {
				log.Fatalln(err)
			}

			block = block.WithSignature(sig)

			bp.insertBlock(*block)
		}
	}
}
