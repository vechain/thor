package subscriptions

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/tx"
)

type output struct {
	txIndex int
	*tx.Output
}

func outputs(chain *chain.Chain, blks []*block.Block) ([]*output, error) {
	result := []*output{}
	for _, blk := range blks {
		receipts, err := chain.GetBlockReceipts(blk.Header().ID())
		if err != nil {
			return nil, err
		}
		for i, receipt := range receipts {
			for _, v := range receipt.Outputs {
				result = append(result, &output{i, v})
			}
		}
	}
	return result, nil
}
