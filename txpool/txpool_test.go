package txpool_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

const (
	emptyRootHash = "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
	testAddress   = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
	testPrivHex   = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"
)

func TestTxPool(t *testing.T) {
	db, _ := lvldb.NewMem()
	chain := chain.New(db)
	c := state.NewCreator(db)
	bl, _, err := genesis.Dev.Build(c)
	if err != nil {
		t.Fatal(err)
	}
	chain.WriteGenesis(bl)
	best, _ := chain.GetBestBlock()
	blk := new(block.Builder).
		ParentID(best.Header().ID()).
		StateRoot(best.Header().StateRoot()).
		Build()
	if _, err := chain.AddBlock(blk, true); err != nil {
		t.Fatal(err)
	}
	address, _ := thor.ParseAddress(testAddress)
	pool := txpool.New(chain, c)
	count := 10
	ch := make(chan *tx.Transaction, count)
	sub := pool.SubscribeNewTransaction(ch)
	defer sub.Unsubscribe()
	dependsOn := thor.BytesToHash([]byte("depend"))
	for i := 0; i < count; i++ {
		cla := tx.NewClause(&address).WithValue(big.NewInt(10 + int64(i))).WithData(nil)
		tx := new(tx.Builder).
			GasPriceCoef(1).
			Gas(1000000).
			Clause(cla).
			DependsOn(&dependsOn).
			Nonce(1).
			Build()
		key, err := crypto.HexToECDSA(testPrivHex)
		if err != nil {
			t.Fatal(err)
		}
		sig, err := crypto.Sign(tx.SigningHash().Bytes(), key)
		if err != nil {
			t.Errorf("Sign error: %s", err)
		}
		tx = tx.WithSignature(sig)
		if err := pool.Add(tx); err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < count; i++ {
		select {
		case t1 := <-ch:
			fmt.Println(i, t1)
		case <-time.After(time.Second):
			fmt.Println("event not fired", i)
		}
	}
}
