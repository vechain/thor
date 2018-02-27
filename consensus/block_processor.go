package consensus

import (
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type packerContext struct {
	txDelayFunc func(tx.BlockRef) (uint64, error)
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

// ProcessBlock can execute all transactions in a block.
func (bp *blockProcessor) process(blk *block.Block, preHeader *block.Header) (*big.Int, error) {
	pc := &packerContext{
		txDelayFunc: func(blockRef tx.BlockRef) (uint64, error) {
			return MeasureTxDelay(blockRef, preHeader, bp.chain)
		},
		rewardRatio: builtin.Params.Get(bp.rt.State(), thor.KeyRewardRatio)}

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

	baseGasPrice := builtin.Params.Get(bp.rt.State(), thor.KeyBaseGasPrice)
	big2 := big.NewInt(2)
	lowGasPrice := new(big.Int).Div(baseGasPrice, big2)
	highGasPrice := new(big.Int).Mul(baseGasPrice, big2)
	gasPrice := transaction.GasPrice()
	if gasPrice.Cmp(highGasPrice) > 0 || gasPrice.Cmp(lowGasPrice) < 0 {
		return nil, nil, errPrice
	}

	receipt, _, err := bp.rt.ExecuteTransaction(transaction)
	if err != nil {
		return nil, nil, err
	}

	return receipt, CalcReward(transaction, receipt.GasUsed, pc.rewardRatio, bp.rt.BlockTime(), delay), nil
}
