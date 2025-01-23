// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
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
	if err := repo.AddBlock(blk, receipts, 0, true); err != nil {
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
			Expiration(1000).
			Gas(21000).
			Nonce(uint64(datagen.RandInt())).
			Clause(cla).
			BlockRef(tx.NewBlockRef(0)).
			Build(),
		genesis.DevAccounts()[addressNumber].PrivateKey,
	)
}

func TestPendingTx_NoWriteAfterUnsubscribe(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	txPool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	})

	p := newPendingTx(txPool)
	txCh := make(chan *tx.Transaction, txQueueSize)

	// Subscribe and then unsubscribe
	p.Subscribe(txCh)
	p.Unsubscribe(txCh)

	done := make(chan struct{})
	// Attempt to write a new transaction
	trx := createTx(thorChain.Repo(), 0)
	assert.NotPanics(t, func() {
		p.dispatch(trx, done) // dispatch should not panic after unsubscribe
	}, "Dispatching after unsubscribe should not panic")

	select {
	case <-txCh:
		t.Fatal("Channel should not receive new transactions after unsubscribe")
	default:
		t.Log("No transactions sent to unsubscribed channel, as expected")
	}
}

func TestPendingTx_UnsubscribeOnWebSocketClose(t *testing.T) {
	// Arrange
	thorChain := initChain(t)
	txPool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{
		Limit:           100,
		LimitPerAccount: 16,
		MaxLifetime:     time.Hour,
	})

	// Subscriptions setup
	sub := New(thorChain.Repo(), []string{"*"}, 100, txPool, false)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		utils.WrapHandlerFunc(sub.handlePendingTransactions)(w, r)
	}))
	defer server.Close()

	require.Equal(t, len(sub.pendingTx.listeners), 0)

	// Connect as WebSocket client
	url := "ws" + server.URL[4:] + "/txpool"
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	assert.NoError(t, err)
	defer ws.Close()

	// Add a transaction
	trx := createTx(thorChain.Repo(), 0)
	txPool.AddLocal(trx)

	// Wait to receive transaction
	time.Sleep(500 * time.Millisecond)
	sub.pendingTx.mu.Lock()
	require.Equal(t, len(sub.pendingTx.listeners), 1)
	sub.pendingTx.mu.Unlock()

	// Simulate WebSocket closure
	ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	ws.Close()

	// Wait for cleanup
	time.Sleep(5 * time.Second)

	// Assert cleanup
	sub.pendingTx.mu.Lock()
	require.Equal(t, len(sub.pendingTx.listeners), 0)
	sub.pendingTx.mu.Unlock()
}
