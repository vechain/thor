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

// HandleBlockChunk to handle a chunk of blocks downloaded in sync process.
type HandleBlockChunk func(ctx context.Context, chunk []*block.Block) error
