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

func TestNode_TxStashLoop_TxEvents(t *testing.T) {
	tests := []struct {
		name       string
		txEvent    *txpool.TxEvent
		beforeFunc func(t *testing.T, values map[string]any)
		assertFunc func(t *testing.T, values map[string]any)
	}{
		{
			name: "Send executable Tx Event to txStashLoop",
			txEvent: &txpool.TxEvent{
				Tx:         newTx(tx.TypeLegacy),
				Executable: &[]bool{true}[0],
			},
			assertFunc: func(t *testing.T, values map[string]any) {
				assert.True(t, values["processed"].(bool), "TxEvent should be processed")
				node := values["node"].(*Node)
				log := values["logs"].(string)
				assert.True(t, len(node.txStash.LoadAll()) == 0, "TxStash should be empty")
				assert.Contains(t, log, "received executable tx signal", "Log should contain executable tx signal")
			},
		},
		{
			name: "Send unexecutable Tx Event to txStashLoop",
			txEvent: &txpool.TxEvent{
				Tx:         newTx(tx.TypeLegacy),
				Executable: &[]bool{false}[0],
			},
			assertFunc: func(t *testing.T, values map[string]any) {
				assert.True(t, values["processed"].(bool), "TxEvent should be processed")
				node := values["node"].(*Node)
				log := values["logs"].(string)
				assert.True(t, len(node.txStash.LoadAll()) == 1, "TxStash should contain 1 item")
				assert.Contains(t, log, values["tx"].(*tx.Transaction).ID().String(), "Log should contain executable txid")
			},
		},
		{
			name: "TxStash save error handling",
			txEvent: &txpool.TxEvent{
				Tx:         newTx(tx.TypeLegacy),
				Executable: &[]bool{false}[0],
			},
			beforeFunc: func(t *testing.T, values map[string]any) {
				db := values["db"].(*leveldb.DB)
				if db != nil {
					db.Close() // Close DB to force error on Save
				}
			},
			assertFunc: func(t *testing.T, values map[string]any) {
				assert.True(t, values["processed"].(bool), "TxEvent should be processed")
				log := values["logs"].(string)
				assert.Contains(t, log, "leveldb: closed", "Log should contain stash error")
				assert.Contains(t, log, values["tx"].(*tx.Transaction).ID().String(), "Log should contain executable txid")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := leveldb.Open(storage.NewMemStorage(), nil)
			assert.NoError(t, err)
			defer db.Close()

			node := setupTestNodeForTxstashLoop(db)

			buf, restore := captureLogs()
			defer restore()

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			var processed bool
			done := make(chan bool, 1)

			if tt.beforeFunc != nil {
				tt.beforeFunc(t, map[string]any{
					"node": node,
					"db":   db,
				})
			}

			go func() {
				defer func() { done <- true }()

				// Monitor for processing
				go func() {
					select {
					case node.txCh <- tt.txEvent:
						processed = true
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

			// Verify expectations
			if tt.assertFunc != nil {
				tt.assertFunc(t, map[string]any{
					"processed": processed,
					"node":      node,
					"logs":      buf.String(),
					"tx":        tt.txEvent.Tx,
				})
			}
		})
	}
}
