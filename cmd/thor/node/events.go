package node

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
)

type bestBlockEvent struct {
	*block.Block
}

type packedBlockEvent struct {
	*block.Block
	stage    *state.Stage
	receipts tx.Receipts
}
