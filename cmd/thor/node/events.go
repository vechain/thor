package node

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/tx"
)

type bestBlockEvent struct {
	*block.Block
}

type packedBlockEvent struct {
	*block.Block
	receipts tx.Receipts
}
