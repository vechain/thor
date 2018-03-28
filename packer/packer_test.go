package packer_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type txIterator struct {
	chainTag byte
	i        int
}

var nonce uint64 = uint64(time.Now().UnixNano())

func (ti *txIterator) HasNext() bool {
	return ti.i < 100
}
func (ti *txIterator) Next() *tx.Transaction {
	ti.i++

	accs := genesis.Dev.Accounts()
	a0 := accs[0]
	a1 := accs[1]

	method := builtin.Energy.ABI.MethodByName("transfer")

	data, _ := method.EncodeInput(a1.Address, big.NewInt(1))

	tx := new(tx.Builder).
		ChainTag(ti.chainTag).
		Clause(tx.NewClause(&builtin.Energy.Address).WithData(data)).
		Gas(300000).GasPriceCoef(0).Nonce(nonce).Build()
	nonce++
	sig, _ := crypto.Sign(tx.SigningHash().Bytes(), a0.PrivateKey)
	tx = tx.WithSignature(sig)

	return tx
}

func (ti *txIterator) OnProcessed(txID thor.Hash, err error) {
}

func TestP(t *testing.T) {

	kv, _ := lvldb.New("/tmp/thor", lvldb.Options{})
	defer kv.Close()

	b0, _, _ := genesis.Dev.Build(state.NewCreator(kv))

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
		best, _ := c.GetBestBlock()
		p := packer.New(c, stateCreator, a1.Address, a1.Address)
		_, adopt, commit, err := p.Prepare(best.Header(), uint64(time.Now().Unix()))
		if err != nil {
			t.Fatal(err)
		}
		iter := &txIterator{chainTag: b0.Header().ID()[31]}
		for iter.HasNext() {
			tx := iter.Next()
			adopt(tx)
		}

		blk, _, err := commit(genesis.Dev.Accounts()[0].PrivateKey)
		fmt.Println(consensus.New(c, stateCreator).Consent(blk, uint64(time.Now().Unix()*2)))

		if _, err := c.AddBlock(blk, true); err != nil {
			t.Fatal(err)
		}

		if time.Now().UnixNano() > start+1000*1000*1000*1 {
			break
		}
	}

	best, _ := c.GetBestBlock()
	fmt.Println(best.Header().Number(), best.Header().GasUsed())
	//	fmt.Println(best)
}
