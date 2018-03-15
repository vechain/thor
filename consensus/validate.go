package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	Tx "github.com/vechain/thor/tx"
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
	case !c.validateTransactions(blk):
		return nil, errTransaction
	default:
		return parentHeader, nil
	}
}

func (c *Consensus) validateTransactions(blk *block.Block) bool {
	transactions := blk.Transactions()
	header := blk.Header()

	if len(transactions) == 0 {
		return true
	}

	validTx := make(map[thor.Hash]bool)

	for _, tx := range transactions {
		switch {
		case tx.BlockRef().Number() >= header.Number():
			return false
		case tx.ChainTag() != header.ChainTag():
			return false
		case !c.isTxDependFound(validTx, header, tx):
			return false
		case !c.isTxNotFound(validTx, header, tx):
			return false
		}
		validTx[tx.ID()] = true
	}

	return true
}

func (c *Consensus) isTxNotFound(validTx map[thor.Hash]bool, header *block.Header, tx *Tx.Transaction) bool {
	if _, ok := validTx[tx.ID()]; ok { // 在当前块中找到相同交易
		return false
	}

	_, err := c.chain.LookupTransaction(header.ParentID(), tx.ID())
	return c.chain.IsNotFound(err)
}

func (c *Consensus) isTxDependFound(validTx map[thor.Hash]bool, header *block.Header, tx *Tx.Transaction) bool {
	dependID := tx.DependsOn()
	if dependID == nil { // 不依赖其它交易
		return true
	}

	if _, ok := validTx[*dependID]; ok { // 在当前块中找到依赖
		return true
	}

	_, err := c.chain.LookupTransaction(header.ParentID(), *dependID)
	return err != nil // 在 chain 中找到依赖
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

	targetTime := sched.Schedule(header.Timestamp())
	if !sched.IsTheTime(targetTime) {
		return errSchedule
	}

	updates, score := sched.Updates(targetTime)
	if parentHeader.TotalScore()+score != header.TotalScore() {
		return errTotalScore
	}

	for _, proposer := range updates {
		builtin.Authority.Update(st, proposer.Address, proposer.Status)
	}

	return nil
}
