// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func setupTestNodeForTxstashLoop(db *leveldb.DB) *Node {
	// Create original node
	originalNode := &Node{
		txCh:    make(chan *txpool.TxEvent, 1),
		txStash: newTxStash(db, 100),
	}
	return originalNode
}

func TestNode_TxStashLoop_ExecutableTx_Processing(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node := setupTestNodeForTxstashLoop(db)

	buf, restore := captureLogs()
	defer restore()

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
			node.txStashLoop(ctx)
		}
	}()

	// Allow some time for processing
	time.Sleep(200 * time.Millisecond)

	// Signal completion before reading logs
	cancel()
	<-done

	assert.True(t, len(node.txStash.LoadAll()) == 0, "TxStash should be empty")
	assert.Contains(t, buf.String(), "received executable tx signal", "Log should contain executable tx signal")
}

func TestNode_TxStashLoop_UnexecutableTx_Processing(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node := setupTestNodeForTxstashLoop(db)

	buf, restore := captureLogs()
	defer restore()

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
			node.txStashLoop(ctx)
		}
	}()

	// Allow some time for processing
	time.Sleep(200 * time.Millisecond)

	// Signal completion before reading logs
	cancel()
	<-done

	assert.True(t, len(node.txStash.LoadAll()) == 1, "TxStash should contain 1 item")
	assert.Contains(t, buf.String(), txEvent.Tx.ID().String(), "Log should contain executable txid")
}

func TestNode_TxStashLoop_StashErrorHandling(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	assert.NoError(t, err)
	defer db.Close()

	node := setupTestNodeForTxstashLoop(db)

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
			node.txStashLoop(ctx)
		}
	}()

	// Allow some time for processing
	time.Sleep(200 * time.Millisecond)

	// Signal completion before reading logs
	cancel()
	<-done

	assert.Contains(t, buf.String(), "leveldb: closed", "Log should contain stash error")
	assert.Contains(t, buf.String(), txEvent.Tx.ID().String(), "Log should contain executable txid")
}
