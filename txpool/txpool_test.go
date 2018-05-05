package txpool_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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
	pool := initPool(t)
	defer pool.Stop()
	count := 10
	addTx(t, pool, count)
	pending(t, pool, count)
	dump(t, pool, count)
}

func pending(t *testing.T, pool *txpool.TxPool, count int) {
	txs := pool.Pending()
	assert.Equal(t, len(txs), count)
}

func dump(t *testing.T, pool *txpool.TxPool, count int) {
	txs := pool.Dump()
	assert.Equal(t, len(txs), count)
}

func addTx(t *testing.T, pool *txpool.TxPool, count int) {

	ch := make(chan *tx.Transaction, count)
	sub := pool.SubscribeNewTransaction(ch)
	defer sub.Unsubscribe()
	address := thor.BytesToAddress([]byte("addr"))
	for i := 0; i < count; i++ {
		cla := tx.NewClause(&address).WithValue(big.NewInt(10 + int64(i))).WithData(nil)
		tx := new(tx.Builder).
			GasPriceCoef(1).
			Gas(1000000).
			Expiration(100).
			Clause(cla).
			Nonce(1).
			Build()
		key, err := crypto.HexToECDSA(testPrivHex)
		if err != nil {
			t.Fatal(err)
		}
		sig, err := crypto.Sign(tx.SigningHash().Bytes(), key)
		if err != nil {
			t.Fatal(err)
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
			t.Errorf("event not fired")
		}
	}
}
func initPool(t *testing.T) *txpool.TxPool {
	db, _ := lvldb.NewMem()
	stateC := state.NewCreator(db)
	gen, err := genesis.NewDevnet()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := gen.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := chain.New(db, b)
	best := c.BestBlock()
	blk := new(block.Builder).
		ParentID(best.Header().ID()).
		StateRoot(best.Header().StateRoot()).
		Build()
	if _, err := c.AddBlock(blk, nil, true); err != nil {
		t.Fatal(err)
	}
	return txpool.New(c, stateC)
}
