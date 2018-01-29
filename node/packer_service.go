package node

import (
	"context"
	"crypto/ecdsa"
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

func packerService(ctx context.Context, bestBlockUpdate chan bool, bp *blockPool, chain *chain.Chain, pk *packer.Packer, privateKey *ecdsa.PrivateKey) {
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

		log.Printf("[packer]: %v\n", time.Duration(ts-now)*time.Second)
		target := time.After(time.Duration(ts-now) * time.Second)

		select {
		case <-ctx.Done():
			log.Printf("[packer]: packerService exit\n")
			return
		case <-bestBlockUpdate:
			log.Printf("[packer]: best block update\n")
			continue
		case <-target:
			block, _, err := pack(&fakeTxFeed{})
			if err != nil {
				log.Fatalln(err)
			}

			sig, err := crypto.Sign(block.Header().SigningHash().Bytes(), privateKey)
			if err != nil {
				log.Fatalln(err)
			}

			block = block.WithSignature(sig)

			// 在分布式环境下:
			// if err = chain.AddBlock(block, true); err != nil {
			// 	log.Fatalln(err)
			// }
			// 在测试环境下:
			bp.insertBlock(*block)
			log.Printf("[packer]: build a block, and sleep 5s\n")
			time.Sleep(5 * time.Second)
		}
	}
}
