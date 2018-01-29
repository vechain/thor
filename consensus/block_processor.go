package consensus

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/tx"
)

type packerContext struct {
	txDelayFunc func(tx.BlockRef) (uint32, error)
	rewardRatio *big.Int
}

type blockProcessor struct {
	rt    *runtime.Runtime
	chain *chain.Chain
}

func newBlockProcessor(rt *runtime.Runtime, chain *chain.Chain) *blockProcessor {
	return &blockProcessor{
		rt:    rt,
		chain: chain}
}

func (bp *blockProcessor) preparePickerContext(blk *block.Block) (*packerContext, error) {
	txDelay := func(blockRef tx.BlockRef) (uint32, error) {
		return MeasureTxDelay(blockRef, blk.ParentID(), bp.chain)
	}

	output := handleClause(bp.rt,
		contracts.Params.PackGet(contracts.ParamRewardRatio))

	if output.VMErr != nil {
		return nil, errors.Wrap(output.VMErr, "reward percentage")
	}

	return &packerContext{
		txDelayFunc: txDelay,
		rewardRatio: contracts.Params.UnpackGet(output.Value)}, nil
}

// ProcessBlock can execute all transactions in a block.
func (bp *blockProcessor) process(blk *block.Block) (*big.Int, error) {
	pc, err := bp.preparePickerContext(blk)
	if err != nil {
		return nil, err
	}

	receipts, totalGasUsed, totalReward, err := bp.processTransactions(pc, blk.Transactions(), nil, 0, big.NewInt(0))
	if err != nil {
		return nil, err
	}

	header := blk.Header()
	switch {
	case header.ReceiptsRoot() != receipts.RootHash():
		return nil, errReceiptsRoot
	case header.GasUsed() != totalGasUsed:
		return nil, errGasUsed
	default:
		return totalReward, nil
	}
}

func (bp *blockProcessor) processTransactions(
	pc *packerContext,
	transactions tx.Transactions,
	receipts tx.Receipts,
	totalGasUsed uint64,
	totalReward *big.Int) (tx.Receipts, uint64, *big.Int, error) {

	length := len(transactions)
	if length == 0 {
		return receipts, totalGasUsed, totalReward, nil
	}

	receipt, reward, err := bp.processTransaction(pc, transactions[0])
	if err != nil {
		return nil, 0, nil, err
	}

	return bp.processTransactions(pc, transactions[1:len(transactions)],
		append(receipts, receipt),
		totalGasUsed+receipt.GasUsed,
		new(big.Int).Add(totalReward, reward))
}

func (bp *blockProcessor) processTransaction(pc *packerContext, transaction *tx.Transaction) (*tx.Receipt, *big.Int, error) {
	delay, err := pc.txDelayFunc(transaction.BlockRef())
	if err != nil {
		return nil, nil, err
	}

	receipt, _, err := bp.rt.ExecuteTransaction(transaction)
	if err != nil {
		return nil, nil, err
	}

	return receipt, CalcReward(transaction, receipt.GasUsed, pc.rewardRatio, bp.rt.BlockNumber(), delay), nil
}
