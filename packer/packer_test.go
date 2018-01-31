package packer_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
)

type txFeed struct {
	i int
}

var nonce uint64 = uint64(time.Now().UnixNano())

func (tf *txFeed) Next() *tx.Transaction {
	if tf.i < 100 {
		accs := genesis.Dev.Accounts()
		a0 := accs[0]
		a1 := accs[1]

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

func (tf *txFeed) MarkTxBad(tx *tx.Transaction) {

}

func TestP(t *testing.T) {
	kv, _ := lvldb.New("/tmp/thor", lvldb.Options{})
	defer kv.Close()

	b0, _ := genesis.Dev.Build(state.NewCreator(kv))

	c := chain.New(kv)
	c.WriteGenesis(b0)
	a1 := genesis.Dev.Accounts()[0]

	start := time.Now().UnixNano()
	stateCreator := state.NewCreator(kv)
	// f, err := os.Create("/tmp/ppp")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()
	for {
		parent, err := c.GetBestBlock()
		if err != nil {
			t.Fatal(err)
		}
		p := packer.New(a1.Address, a1.Address, c, stateCreator)
		_, pack, err := p.Prepare(parent.Header(), uint64(time.Now().Unix()))
		if err != nil {
			t.Fatal(err)
		}
		blk, receipts, err := pack(&txFeed{})
		if err := c.AddBlock(blk, true); err != nil {
			t.Fatal(err)
		}
		_, _, _ = blk, receipts, err
		if err != nil {
			t.Fatal(err)
		}

		if time.Now().UnixNano() > start+1000*1000*1000*1 {
			break
		}
	}

	best, _ := c.GetBestBlock()
	fmt.Println(best.Header().Number(), best.Header().GasUsed())
	fmt.Println(best)

}
