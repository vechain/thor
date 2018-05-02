package comm

import (
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

// HandleBlockBatch to handle a batch of blocks downloaded in sync process.
type HandleBlockBatch func(batch []*block.Block) error
