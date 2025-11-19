// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"context"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// LogDB defines the interface for log database operations.
type LogDB interface {
	// FilterEvents filters events based on the given criteria.
	FilterEvents(ctx context.Context, filter *EventFilter) ([]*Event, error)

	// FilterTransfers filters transfers based on the given criteria.
	FilterTransfers(ctx context.Context, filter *TransferFilter) ([]*Transfer, error)

	// NewestBlockID returns the newest written block ID.
	NewestBlockID() (thor.Bytes32, error)

	// HasBlockID checks if the given block ID related logs were written.
	HasBlockID(id thor.Bytes32) (bool, error)

	// NewWriter creates a log writer.
	NewWriter() Writer

	// NewWriterSyncOff creates a log writer with 'pragma synchronous = off'.
	NewWriterSyncOff() Writer

	// Path returns the database path.
	Path() string

	// Close closes the log database.
	Close() error
}

// Writer defines the interface for transactional log writing operations.
type Writer interface {
	// Write writes all logs of the given block.
	Write(b *block.Block, receipts tx.Receipts) error

	// Commit commits accumulated logs.
	Commit() error

	// Rollback rollbacks all uncommitted logs.
	Rollback() error

	// Truncate truncates the database by deleting logs after blockNum (included).
	Truncate(blockNum uint32) error

	// UncommittedCount returns the count of uncommitted logs.
	UncommittedCount() int
}
