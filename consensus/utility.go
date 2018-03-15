package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

func checkState(state *state.State, header *block.Header) error {
	if stateRoot, err := state.Stage().Hash(); err == nil {
		if header.StateRoot() != stateRoot {
			return errStateRoot
		}
	} else {
		return err
	}
	return nil
}

func AddBlock(c comm.Comm, ch *chain.Chain, blk *block.Block, isTrunk bool) error {
	if err := ch.AddBlock(blk, isTrunk); err != nil {
		return err
	}
	c.BroadcastBlk(blk)
	return nil
}

func AddTxPool(c comm.Comm, pool *txpool.TxPool, tx *tx.Transaction) error {
	if err := pool.Add(tx); err != nil {
		return err
	}
	c.BroadcastTx(tx)
	return nil
}
