package subscriptions

import (
	"context"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type TransferSub struct {
	chain  *chain.Chain
	filter *TransferFilter
	bs     *BlockSub
}

func NewTransferSub(chain *chain.Chain, fromBlock thor.Bytes32, filter *TransferFilter) *TransferSub {
	return &TransferSub{
		chain:  chain,
		filter: filter,
		bs:     NewBlockSub(chain, fromBlock),
	}
}

func (ts *TransferSub) Read(ctx context.Context) ([]*Transfer, []*Transfer, error) {
	blkChanges, blkRemoves, err := ts.bs.Read(ctx)
	if err != nil {
		return nil, nil, err
	}

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
	outputs, err := extractOutputs(ts.chain, blks)
	if err != nil {
		return nil, err
	}

	result := []*Transfer{}
	for _, output := range outputs {
		for _, transfer := range output.Transfers {
			if ts.filter.match(transfer, output.origin) {
				result = append(result, newTransfer(output.header, output.origin, output.tx, transfer))
			}
		}
	}

	return result, nil
}
