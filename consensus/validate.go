package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func (c *Consensus) validateBlock(blk *block.Block, nowTime uint64) (*block.Header, error) {
	header := blk.Header()
	parentHeader, err := c.chain.GetBlockHeader(header.ParentID())
	if err != nil {
		if c.chain.IsNotFound(err) {
			return nil, errParentNotFound
		}
		return nil, err
	}

	gasLimit := header.GasLimit()

	// Signer and IntrinsicGas will be validate in runtime.
	switch {
	case parentHeader.Timestamp() >= header.Timestamp():
		return nil, errTimestamp
	case header.Timestamp() > nowTime+thor.BlockInterval:
		return nil, errDelay
	case !block.GasLimit(gasLimit).IsValid(parentHeader.GasLimit()):
		return nil, errGasLimit
	case header.GasUsed() > gasLimit:
		return nil, errGasUsed
	case header.TxsRoot() != blk.Transactions().RootHash():
		return nil, errTxsRoot
	}

	if err := c.validateTransactions(blk); err != nil {
		return nil, errors.Wrap(err, "bad tx")
	}
	return parentHeader, nil
}

func (c *Consensus) validateTransactions(blk *block.Block) error {
	transactions := blk.Transactions()
	header := blk.Header()

	if len(transactions) == 0 {
		return nil
	}

	passed := make(map[thor.Hash]struct{})

	findTx := func(txID thor.Hash) (bool, error) {
		if _, ok := passed[txID]; ok {
			return true, nil
		}
		_, err := c.chain.LookupTransaction(blk.Header().ParentID(), txID)
		if err != nil {
			if c.chain.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	for _, tx := range transactions {
		switch {
		case tx.BlockRef().Number() >= header.Number():
			return errors.New("invalid block ref")
		case tx.ChainTag() != header.ChainTag():
			return errors.New("chain tag mismatch")
		case tx.HasReservedFields():
			return errors.New("reserved fields not empty")
		}

		// check if tx already appeared in old blocks, or passed map
		if found, err := findTx(tx.ID()); err != nil {
			return err
		} else if found {
			return errors.New("duplicated tx")
		}

		// check depended tx
		if dep := tx.DependsOn(); dep != nil {
			if found, err := findTx(*dep); err != nil {
				return err
			} else if !found {
				return errors.New("tx dep not found")
			}
		}
		passed[tx.ID()] = struct{}{}
	}
	return nil
}

func (c *Consensus) validateProposer(header *block.Header, parentHeader *block.Header, st *state.State) error {
	signer, err := header.Signer()
	if err != nil {
		return err
	}

	sched, err := poa.NewScheduler(signer, builtin.Authority.All(st), parentHeader.Number(), parentHeader.Timestamp())
	if err != nil {
		return err
	}

	if !sched.IsTheTime(header.Timestamp()) {
		return errSchedule
	}

	updates, score := sched.Updates(header.Timestamp())
	if parentHeader.TotalScore()+score != header.TotalScore() {
		return errTotalScore
	}

	for _, proposer := range updates {
		builtin.Authority.Update(st, proposer.Address, proposer.Status)
	}

	return nil
}
