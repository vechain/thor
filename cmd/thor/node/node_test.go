// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logsdb/sqlite3"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// Mock implementations for testing
type mockTxPool struct {
	txpoolChan chan *txpool.TxEvent
	txFeed     event.Feed
	mu         sync.Mutex
}

func (m *mockTxPool) Fill(txs tx.Transactions) {
}

func (m *mockTxPool) Add(newTx *tx.Transaction) error {
	return nil
}

func (m *mockTxPool) SubscribeTxEvent(ch chan *txpool.TxEvent) event.Subscription {
	m.mu.Lock()
	m.txpoolChan = ch
	m.mu.Unlock()
	return m.txFeed.Subscribe(ch)
}

func (m *mockTxPool) getTxChannel() chan *txpool.TxEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.txpoolChan
}

func (m *mockTxPool) Executables() tx.Transactions {
	return tx.Transactions{}
}

func (m *mockTxPool) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool {
	return true
}

func (m *mockTxPool) Close() {}

func (m *mockTxPool) AddLocal(newTx *tx.Transaction) error {
	return nil
}

func (m *mockTxPool) Get(id thor.Bytes32) *tx.Transaction {
	return nil
}

func (m *mockTxPool) StrictlyAdd(newTx *tx.Transaction) error {
	return nil
}

func (m *mockTxPool) Dump() tx.Transactions {
	return tx.Transactions{}
}

func (m *mockTxPool) Len() int {
	return 0
}

func createDevAccounts(t *testing.T, accountNo int) []genesis.DevAccount {
	var accs []genesis.DevAccount

	for range accountNo {
		pk, err := crypto.GenerateKey()
		require.NoError(t, err)
		addr := crypto.PubkeyToAddress(pk.PublicKey)
		accs = append(accs, genesis.DevAccount{Address: thor.Address(addr), PrivateKey: pk})
	}

	return accs
}

func testNode(t *testing.T) (*Node, error) {
	// create state accounts
	accounts := createDevAccounts(t, 5)

	// create test db
	db := muxdb.NewMem()

	memLogdb, err := sqlite3.NewMem()
	require.NoError(t, err)

	// Initialize the test chain and dependencies
	thorChain, err := createChain(db, accounts)
	require.NoError(t, err)

	//
	tempDir := t.TempDir()

	proposer := &accounts[0]

	engine, err := bft.NewEngine(thorChain.Repo(), thorChain.Database(), thorChain.GetForkConfig(), proposer.Address)
	require.NoError(t, err)

	masterAddr := &Master{
		PrivateKey: proposer.PrivateKey,
	}

	mockComm := &mockCommunicator{peerCount: 1}
	mockTxPool := &mockTxPool{}

	node := New(
		masterAddr,
		thorChain.Repo(),
		engine,
		thorChain.Stater(),
		memLogdb,
		mockTxPool,
		tempDir,
		mockComm,
		&thor.NoFork,
		Options{
			SkipLogs:         true,
			MinTxPriorityFee: 0,
			TargetGasLimit:   10_000_000,
		},
		consensus.New(thorChain.Repo(), thorChain.Stater(), &thor.NoFork),
		packer.New(thorChain.Repo(), thorChain.Stater(), masterAddr.Address(), masterAddr.Beneficiary, &thor.NoFork, 10_000_000),
	)
	return node, nil
}

// captureLogs temporarily replaces the global root logger with one that
// writes into an in-memory buffer and returns the buffer and a restore func.
// Use restore() in a defer to ensure the original logger is restored.
func captureLogs() (*bytes.Buffer, func()) {
	buf := new(bytes.Buffer)
	old := log.Root()
	h := log.JSONHandler(buf)
	newLogger := log.NewLogger(h)
	log.SetDefault(newLogger)
	return buf, func() { log.SetDefault(old) }
}

func TestNode_Run(t *testing.T) {
	node, err := testNode(t)
	assert.NoError(t, err, "Failed to create test node")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() {
		err := node.Run(ctx)
		assert.NoError(t, err, "Node run should not return an error")
	}()

	// Allow some time for the node to start
	time.Sleep(100 * time.Millisecond)
}

func TestNode_GuardBlockProcessing_NormalNewBlock(t *testing.T) {
	node, err := testNode(t)
	assert.NoError(t, err, "Failed to create test node")

	node.maxBlockNum = uint32(1000)
	newBlockNum := uint32(1001)
	err = node.guardBlockProcessing(newBlockNum, func(conflicts uint32) error {
		// mock process function and return no error
		return nil
	})
	assert.NoError(t, err, "Normal new block should be processed without error")
	assert.Equal(t, node.maxBlockNum, newBlockNum, "maxBlockNum should be updated to new block number")
}

func TestNode_GuardBlockProcessing_FutureBlock(t *testing.T) {
	node, err := testNode(t)
	assert.NoError(t, err, "Failed to create test node")

	node.maxBlockNum = uint32(1000)
	err = node.guardBlockProcessing(1005, func(conflicts uint32) error {
		// mock process function and return no error
		return nil
	})
	assert.ErrorContains(t, err, errBlockTemporaryUnprocessable.Error(), "Future block should return temporary unprocessable error")
}

func TestNode_GuardBlockProcessing_KnownBlock(t *testing.T) {
	node, err := testNode(t)
	assert.NoError(t, err, "Failed to create test node")

	node.maxBlockNum = uint32(1000)
	newBlockNum := uint32(980)
	err = node.guardBlockProcessing(newBlockNum, func(conflicts uint32) error {
		// mock process function and return errKnownBlock
		return errKnownBlock
	})
	assert.ErrorContains(t, err, errKnownBlock.Error(), "Future block should return temporary unprocessable error")
	assert.Equal(t, node.maxBlockNum, uint32(1000), "maxBlockNum should remain unchanged for old block")
}

func TestNode_HandleBlockStream_SendNormalBlock(t *testing.T) {
	node, err := testNode(t)
	assert.NoError(t, err, "Failed to create test node")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	blockStream := make(chan *block.Block, 1) // Make buffered to avoid blocking

	done := make(chan bool, 1)

	go func() {
		defer func() { done <- true }()
		err := node.handleBlockStream(ctx, blockStream)
		assert.NoError(t, err, "handleBlockStream should not return an error")
	}()

	// Mock normal block and send it
	parentBlock := node.repo.BestBlockSummary()
	select {
	case blockStream <- block.Compose(parentBlock.Header, nil):
		// Successfully sent
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout sending block to stream")
	}

	// Close the channel to signal end of stream
	close(blockStream)

	// Wait for the handler to complete
	select {
	case <-done:
		// Handler completed successfully
	case <-time.After(400 * time.Millisecond):
		t.Fatal("Timeout waiting for handler to complete")
	}
}

func setupTestNodeForTxstashLoop(db *leveldb.DB) (*Node, *txStash, *mockTxPool) {
	txpool := &mockTxPool{}
	originalNode := &Node{txPool: txpool}
	return originalNode, newTxStash(db, 100), txpool
}

func TestNode_TxStashLoop_FillTxPool(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node, stash, _ := setupTestNodeForTxstashLoop(db)

	txs := tx.Transactions{
		newTx(tx.TypeLegacy),
		newTx(tx.TypeLegacy),
	}

	for _, tx := range txs {
		err := stash.Save(tx)
		assert.NoError(t, err, "Failed to save transaction to stash")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start the txStashLoop in a goroutine
	node.txStashLoop(ctx, stash)
	assert.Equal(t, len(txs), len(stash.LoadAll()), "All transactions should remain in stash after fill attempt")
}

func TestNode_TxStashLoop_ExecutableTx_Processing(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node, stash, pool := setupTestNodeForTxstashLoop(db)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	txEvent := txpool.TxEvent{
		Tx:         newTx(tx.TypeLegacy),
		Executable: &[]bool{true}[0], // Transaction is executable
	}

	// Start the txStashLoop in a goroutine
	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.txStashLoop(ctx, stash)
	}()

	// Wait for the txStashLoop to set up the subscription and get the channel
	var txChan chan *txpool.TxEvent
	for range 10 {
		time.Sleep(10 * time.Millisecond)
		if txChan = pool.getTxChannel(); txChan != nil {
			break
		}
	}

	if txChan == nil {
		t.Fatal("Timeout waiting for tx channel to be established")
	}

	go func() {
		select {
		case txChan <- &txEvent:
			// Successfully sent
		case <-ctx.Done():
			// Context cancelled
		}
	}()

	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for txStashLoop to complete")
	}

	stashedTxs := stash.LoadAll()
	assert.Equal(t, 0, len(stashedTxs), "Executable transactions should not be stashed")
}

func TestNode_TxStashLoop_UnexecutableTx_Processing(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node, stash, pool := setupTestNodeForTxstashLoop(db)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	txEvent := txpool.TxEvent{
		Tx:         newTx(tx.TypeLegacy),
		Executable: &[]bool{false}[0], // Transaction is not executable
	}

	// Start the txStashLoop in a goroutine
	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.txStashLoop(ctx, stash)
	}()

	// Wait for the txStashLoop to set up the subscription and get the channel
	var txChan chan *txpool.TxEvent
	for range 10 {
		time.Sleep(10 * time.Millisecond)
		if txChan = pool.getTxChannel(); txChan != nil {
			break
		}
	}

	if txChan == nil {
		t.Fatal("Timeout waiting for tx channel to be established")
	}

	go func() {
		select {
		case txChan <- &txEvent:
			// Successfully sent
		case <-ctx.Done():
			// Context cancelled
		}
	}()

	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for txStashLoop to complete")
	}

	stashedTxs := stash.LoadAll()
	assert.Equal(t, 1, len(stashedTxs), "Unexecutable transactions should be stashed")
	assert.Equal(t, txEvent.Tx.ID(), stashedTxs[0].ID(), "Stashed transaction should match the unexecutable transaction sent")
}

func TestNode_TxStashLoop_StashErrorHandling(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)

	buf, restore := captureLogs()
	defer restore()

	node, stash, pool := setupTestNodeForTxstashLoop(db)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	txEvent := txpool.TxEvent{
		Tx:         newTx(tx.TypeLegacy),
		Executable: &[]bool{false}[0], // Transaction is not executable
	}

	done := make(chan bool, 1)
	go func() {
		defer func() { done <- true }()
		node.txStashLoop(ctx, stash)
	}()

	var txChan chan *txpool.TxEvent
	for range 10 {
		time.Sleep(10 * time.Millisecond)
		if txChan = pool.getTxChannel(); txChan != nil {
			break
		}
	}

	if txChan == nil {
		t.Fatal("Timeout waiting for tx channel to be established")
	}

	select {
	case txChan <- &txEvent:
		// Successfully sent
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout sending first transaction event")
	}

	time.Sleep(100 * time.Millisecond)

	// Close DB to force error on subsequent operations
	db.Close()

	select {
	case txChan <- &txEvent:
		// Successfully sent
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout sending second transaction event")
	}

	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for txStashLoop to complete")
	}

	logOutput := buf.String()
	assert.Contains(t, logOutput, "leveldb: closed", "Log should contain stash error")
}
