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

func NewEventReader(chain *chain.Chain, fromBlock thor.Bytes32, filter *EventFilter) *EventReader {
	return &EventReader{
		chain:       chain,
		filter:      filter,
		blockReader: chain.NewBlockReader(fromBlock),
	}
}

func (er *EventReader) Read() ([]interface{}, error) {
	blocks, err := er.blockReader.Read()
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	outputs, err := extractOutputs(er.chain, blocks)
	if err != nil {
		return nil, err
	}
	result := []interface{}{}
	for _, output := range outputs {
		for _, event := range output.Events {
			if er.filter.match(event) {
				event, err := convertEvent(output.header, output.origin, output.tx, event, output.obsolete)
				if err != nil {
					return nil, err
				}
				result = append(result, event)
			}
		}
	}
	return result, nil
}
