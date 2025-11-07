// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"bytes"
	"context"
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
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// Mock implementations for testing
type mockTxPool struct{}

func (m *mockTxPool) Fill(txs tx.Transactions) {
}

func (m *mockTxPool) Add(newTx *tx.Transaction) error {
	return nil
}

func (m *mockTxPool) SubscribeTxEvent(ch chan *txpool.TxEvent) event.Subscription {
	return &mockSubscription{}
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

	memLogdb, err := logdb.NewMem()
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

func setupTestNodeForTxstashLoop(db *leveldb.DB) (*Node, *txStash) {
	// Create original node
	originalNode := &Node{
		txCh: make(chan *txpool.TxEvent, 1),
	}
	return originalNode, newTxStash(db, 100)
}

func TestNode_TxStashLoop_ExecutableTx_Processing(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node, stash := setupTestNodeForTxstashLoop(db)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)

	txEvent := txpool.TxEvent{
		Tx:         newTx(tx.TypeLegacy),
		Executable: &[]bool{true}[0],
	}

	go func() {
		defer func() { done <- true }()

		// Monitor for processing
		go func() {
			select {
			case node.txCh <- &txEvent:
			case <-ctx.Done():
			}
		}()

		// Run txStashLoop briefly
		select {
		case <-ctx.Done():
		default:
			node.txStashLoop(ctx, stash)
		}
	}()

	// Allow some time for processing
	time.Sleep(200 * time.Millisecond)

	// Signal completion before reading logs
	cancel()
	<-done

	assert.True(t, len(stash.LoadAll()) == 0, "TxStash should be empty")
}

func TestNode_TxStashLoop_UnexecutableTx_Processing(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node, stash := setupTestNodeForTxstashLoop(db)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)

	txEvent := txpool.TxEvent{
		Tx:         newTx(tx.TypeLegacy),
		Executable: &[]bool{false}[0],
	}

	go func() {
		defer func() { done <- true }()

		// Monitor for processing
		go func() {
			select {
			case node.txCh <- &txEvent:
			case <-ctx.Done():
			}
		}()

		// Run txStashLoop briefly
		select {
		case <-ctx.Done():
		default:
			node.txStashLoop(ctx, stash)
		}
	}()

	// Allow some time for processing
	time.Sleep(200 * time.Millisecond)

	// Signal completion before reading logs
	cancel()
	<-done

	assert.True(t, len(stash.LoadAll()) == 1, "TxStash should contain 1 item")
}

func TestNode_TxStashLoop_StashErrorHandling(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node, stash := setupTestNodeForTxstashLoop(db)

	buf, restore := captureLogs()
	defer restore()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan bool, 1)

	txEvent := txpool.TxEvent{
		Tx:         newTx(tx.TypeLegacy),
		Executable: &[]bool{false}[0],
	}

	// Close DB to force error on Save
	db.Close()

	go func() {
		defer func() { done <- true }()

		// Monitor for processing
		go func() {
			select {
			case node.txCh <- &txEvent:
			case <-ctx.Done():
			}
		}()

		// Run txStashLoop briefly
		select {
		case <-ctx.Done():
		default:
			node.txStashLoop(ctx, stash)
		}
	}()

	// Allow some time for processing
	time.Sleep(200 * time.Millisecond)

	// Signal completion before reading logs
	cancel()
	<-done

	assert.Contains(t, buf.String(), "leveldb: closed", "Log should contain stash error")
}
