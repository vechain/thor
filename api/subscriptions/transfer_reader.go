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

func NewTransferReader(chain *chain.Chain, fromBlock thor.Bytes32, filter *TransferFilter) *TransferReader {
	return &TransferReader{
		chain:       chain,
		filter:      filter,
		blockReader: chain.NewBlockReader(fromBlock),
	}
}

func (tr *TransferReader) Read() ([]interface{}, error) {
	blocks, err := tr.blockReader.Read()
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	outputs, err := extractOutputs(tr.chain, blocks)
	if err != nil {
		return nil, err
	}
	result := []interface{}{}
	for _, output := range outputs {
		for _, transfer := range output.Transfers {
			if tr.filter.match(transfer, output.origin) {
				transfer, err := convertTransfer(output.header, output.origin, output.tx, transfer, output.obsolete)
				if err != nil {
					return nil, err
				}
				result = append(result, transfer)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}
