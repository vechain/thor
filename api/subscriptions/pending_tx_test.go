// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func TestPendingTx_Subscribe(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	txPool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	})

	p := newPendingTx(txPool)

	// When initialized, there should be no listeners
	assert.Empty(t, p.listeners, "There should be no listeners when initialized")

	ch := make(chan *tx.Transaction)
	p.Subscribe(ch)

	assert.Contains(t, p.listeners, ch, "Subscribe should add the channel to the listeners")
}

func TestPendingTx_Unsubscribe(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	txPool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	})
	p := newPendingTx(txPool)

	ch := make(chan *tx.Transaction)
	ch2 := make(chan *tx.Transaction)
	p.Subscribe(ch)
	p.Subscribe(ch2)

	p.Unsubscribe(ch)

	assert.NotContains(t, p.listeners, ch, "Unsubscribe should remove the channel from the listeners")
	assert.Contains(t, p.listeners, ch2, "Unsubscribe should not remove other channels")
}

func TestPendingTx_DispatchLoop(t *testing.T) {
	db := muxdb.NewMem()
	gene := genesis.NewDevnet()
	stater := state.NewStater(db)
	b0, _, _, _ := gene.Build(stater)
	repo, _ := chain.NewRepository(db, b0)

	txPool := txpool.New(repo, state.NewStater(db), txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	})
	p := newPendingTx(txPool)

	// Add new block to be in a sync state
	addNewBlock(repo, stater, b0, t)

	// Create a channel to signal the end of the test
	done := make(chan struct{})
	defer close(done)

	// Create a channel to receive the transaction
	txCh := make(chan *tx.Transaction)
	p.Subscribe(txCh)

	// Add a new tx to the mempool
	transaction := createTx(repo, 0)
	txPool.AddLocal(transaction)

	// Start the dispatch loop
	go p.DispatchLoop(done)

	// Wait for the transaction to be dispatched
	select {
	case dispatchedTx := <-txCh:
		assert.Equal(t, dispatchedTx, transaction)
	case <-time.After(time.Second * 2):
		t.Fatal("Timeout waiting for transaction dispatch")
	}

	// Unsubscribe the channel
	p.Unsubscribe(txCh)

	// Add another tx to the mempool
	tx2 := createTx(repo, 1)
	txPool.AddLocal(tx2)

	// Assert that the channel did not receive the second transaction
	select {
	case <-txCh:
		t.Fatal("Received unexpected transaction")
	case <-time.After(time.Second):
		t.Log("No transaction received, which is expected")
	}
}

func addNewBlock(repo *chain.Repository, stater *state.Stater, b0 *block.Block, t *testing.T) {
	packer := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	sum, _ := repo.GetBlockSummary(b0.Header().ID())
	flow, err := packer.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	blk, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddBlock(blk, receipts, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBestBlockID(blk.Header().ID()); err != nil {
		t.Fatal(err)
	}
}

func createTx(repo *chain.Repository, addressNumber uint) *tx.Transaction {
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	return tx.MustSign(
		new(tx.Builder).
			ChainTag(repo.ChainTag()).
			GasPriceCoef(1).
			Expiration(10).
			Gas(21000).
			Nonce(1).
			Clause(cla).
			BlockRef(tx.NewBlockRef(0)).
			Build(),
		genesis.DevAccounts()[addressNumber].PrivateKey,
	)
}
