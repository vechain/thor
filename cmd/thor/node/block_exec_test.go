// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/common/mclock"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Test data constants
var (
	testParentID  = thor.Bytes32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	testTimestamp = uint64(time.Now().Unix())
)

// Helper functions
func createTestBlock(parentID thor.Bytes32) *block.Block {
	return (&block.Builder{}).
		ParentID(parentID).
		Timestamp(testTimestamp).
		Build()
}

func createTestBlockExecContext() *blockExecContext {
	return &blockExecContext{
		prevBest:   &block.Header{},
		newBlock:   createTestBlock(testParentID),
		receipts:   tx.Receipts{},
		stage:      &state.Stage{},
		becomeBest: true,
		conflicts:  0,
		stats:      nil,
		packing:    false,
		startTime:  mclock.Now(),
	}
}

func TestBlockExecContext(t *testing.T) {
	// Arrange
	ctx := createTestBlockExecContext()

	// Act & Assert
	assert.NotNil(t, ctx)
	assert.Equal(t, uint32(0), ctx.conflicts)
	assert.True(t, ctx.becomeBest)
	assert.False(t, ctx.packing)
	assert.NotNil(t, ctx.newBlock)
	assert.NotNil(t, ctx.receipts)
	assert.NotNil(t, ctx.stage)
}

func TestLogWorker_ErrorHandling(t *testing.T) {
	tests := []struct {
		name            string
		workerError     error
		expectSyncError bool
	}{
		{
			name:            "worker error - should return error on sync",
			workerError:     fmt.Errorf("worker error"),
			expectSyncError: true,
		},
		{
			name:            "no worker error - should not return error on sync",
			workerError:     nil,
			expectSyncError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			_, n := getFlowAndNode(t, nil)
			assert.False(t, n.logDBFailed)

			// Act
			n.logWorker.Run(func() error {
				return tt.workerError
			})
			err := n.logWorker.Sync()

			// Assert
			if tt.expectSyncError {
				assert.Error(t, err)
				assert.Equal(t, tt.workerError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
			// Note: logDBFailed is only set in commitBlock method, not directly by Sync()
			assert.False(t, n.logDBFailed)
		})
	}
}

func TestLogWorker_MultipleTasks(t *testing.T) {
	// Test that worker accumulates errors and returns the first one
	_, n := getFlowAndNode(t, nil)

	// Run multiple tasks with errors
	n.logWorker.Run(func() error {
		return fmt.Errorf("first error")
	})
	n.logWorker.Run(func() error {
		return fmt.Errorf("second error")
	})
	n.logWorker.Run(func() error {
		return fmt.Errorf("third error")
	})

	// Sync should return the first error
	err := n.logWorker.Sync()
	assert.Error(t, err)
	assert.Equal(t, "first error", err.Error())
}
