package node

import (
	"github.com/ethereum/go-ethereum/common/mclock"
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
	elapsed  mclock.AbsTime
}
