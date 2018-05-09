package comm

import (
	"context"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/tx"
)

// NewBlockEvent event emitted when received block announcement.
type NewBlockEvent struct {
	*block.Block
}

// NewTransactionEvent event emitted when received transaction announcement.
type NewTransactionEvent struct {
	*tx.Transaction
}

// HandleBlockStream to handle the stream of downloaded blocks in sync process.
type HandleBlockStream func(ctx context.Context, stream <-chan *block.Block) error
