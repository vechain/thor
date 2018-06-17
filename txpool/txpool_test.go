// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	emptyRootHash = "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
	testAddress   = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
)

var (
	c     *chain.Chain
	txID  thor.Bytes32
	nonce int
)

func TestTxPool(t *testing.T) {
	pool := initPool(t)
	defer pool.Close()

	count := 100
	txs := generateTxs(t, count)
	txID = txs[0].ID()

	// test add tx
	if err := pool.Add(txs...); err != nil {
		t.Fatal(err)
	}
	testPending(t, pool, count)

	// test pool quota
	err := pool.Add(generateTxs(t, 1)...)
	assert.Equal(t, err, rejectedTxErr{"quota exceeds limit"})

	// test remove tx
	pool.Remove(txID)
	testPending(t, pool, count-1)

	// test pool quota
	if err := pool.Add(generateTxs(t, 1)...); err != nil {
		t.Fatal(err)
	}
	testPending(t, pool, count)
}

func testPending(t *testing.T, pool *TxPool, count int) {
	txs := pool.Pending(true)
	assert.Equal(t, len(txs), count)
}

func generateTxs(t *testing.T, count int) tx.Transactions {
	txs := make(tx.Transactions, count, count)
	address := thor.BytesToAddress([]byte("addr"))
	for i := 0; i < count; i++ {
		cla := tx.NewClause(&address).WithValue(big.NewInt(10 + int64(nonce))).WithData(nil)
		tx := new(tx.Builder).
			GasPriceCoef(1).
			Gas(1000000).
			Expiration(100).
			Clause(cla).
			Nonce(1).
			ChainTag(c.Tag()).
			Build()
		sig, err := crypto.Sign(tx.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		txs[i] = tx.WithSignature(sig)
		nonce++
	}
	return txs
}

func initPool(t *testing.T) *TxPool {
	db, _ := lvldb.NewMem()
	stateC := state.NewCreator(db)
	gen := genesis.NewDevnet()
	b, _, err := gen.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	c, _ = chain.New(db, b)
	best := c.BestBlock()
	blk := new(block.Builder).
		ParentID(best.Header().ID()).
		StateRoot(best.Header().StateRoot()).
		Build()
	if _, err := c.AddBlock(blk, nil); err != nil {
		t.Fatal(err)
	}
	return New(c, stateC)
}
