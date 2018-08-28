package subscriptions

import (
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type EventReader struct {
	chain       *chain.Chain
	filter      *EventFilter
	blockReader chain.BlockReader
}

func NewEventReader(chain *chain.Chain, position thor.Bytes32, filter *EventFilter) *EventReader {
	return &EventReader{
		chain:       chain,
		filter:      filter,
		blockReader: chain.NewBlockReader(position),
	}
}

func (er *EventReader) Read() ([]interface{}, error) {
	blocks, err := er.blockReader.Read()
	if err != nil {
		return nil, err
	}

	var msgs []interface{}
	for _, block := range blocks {
		receipts, err := er.chain.GetBlockReceipts(block.Header().ID())
		if err != nil {
			return nil, err
		}
		txs := block.Transactions()
		for i, receipt := range receipts {
			for _, output := range receipt.Outputs {
				for _, event := range output.Events {
					if er.filter.Match(event) {
						msg, err := convertEvent(block.Header(), txs[i], event, block.Obsolete)
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
