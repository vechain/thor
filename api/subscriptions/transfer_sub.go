package subscriptions

import (
	"context"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
)

type TransferSub struct {
	chain  *chain.Chain
	filter *TransferFilter
	bs     *BlockSub
}

func NewTransferSub(chain *chain.Chain, filter *TransferFilter) *TransferSub {
	return &TransferSub{
		chain:  chain,
		filter: filter,
		bs:     NewBlockSub(chain, filter.FromBlock),
	}
}

func (ts *TransferSub) Read(ctx context.Context) ([]*Transfer, []*Transfer, error) {
	blkChanges, blkRemoves, err := ts.bs.Read(ctx)
	if err != nil {
		return nil, nil, err
	}
	ts.filter.FromBlock = ts.bs.fromBlock

	transferChanges, err := ts.filterTransfer(blkChanges)
	if err != nil {
		return nil, nil, err
	}

	transferRemoves, err := ts.filterTransfer(blkRemoves)
	if err != nil {
		return nil, nil, err
	}

	return transferChanges, transferRemoves, nil
}

func (ts *TransferSub) filterTransfer(blks []*block.Block) ([]*Transfer, error) {
	result := []*Transfer{}
	for _, blk := range blks {
		receipts, err := ts.chain.GetBlockReceipts(blk.Header().ID())
		if err != nil {
			return nil, err
		}

		for i, receipt := range receipts {
			for _, output := range receipt.Outputs {
				for _, transfer := range output.Transfers {
					tx := blk.Transactions()[i]
					origin, err := tx.Signer()
					if err != nil {
						return nil, err
					}
					if ts.filter.match(transfer, origin) {
						result = append(result, newTransfer(blk.Header(), origin, tx, transfer))
					}
				}
			}
		}
	}
	return result, nil
}
