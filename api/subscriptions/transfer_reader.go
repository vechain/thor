package subscriptions

import (
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type TransferReader struct {
	chain       *chain.Chain
	filter      *TransferFilter
	blockReader chain.BlockReader
}

func NewTransferReader(chain *chain.Chain, position thor.Bytes32, filter *TransferFilter) *TransferReader {
	return &TransferReader{
		chain:       chain,
		filter:      filter,
		blockReader: chain.NewBlockReader(position),
	}
}

func (tr *TransferReader) Read() ([]interface{}, error) {
	blocks, err := tr.blockReader.Read()
	if err != nil {
		return nil, err
	}

	var msgs []interface{}
	for _, block := range blocks {
		receipts, err := tr.chain.GetBlockReceipts(block.Header().ID())
		if err != nil {
			return nil, err
		}
		txs := block.Transactions()
		for i, receipt := range receipts {
			for _, output := range receipt.Outputs {
				for _, transfer := range output.Transfers {
					origin, err := txs[i].Signer()
					if err != nil {
						return nil, err
					}
					if tr.filter.Match(transfer, origin) {
						msg, err := convertTransfer(block.Header(), txs[i], transfer, block.Obsolete)
						if err != nil {
							return nil, err
						}
						msgs = append(msgs, msg)
					}
				}
			}
		}
	}
	return msgs, nil
}
