package subscriptions

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type EventSub struct {
	chain  *chain.Chain
	filter *EventFilter
	bs     *BlockSub
}

func NewEventSub(chain *chain.Chain, fromBlock thor.Bytes32, filter *EventFilter) *EventSub {
	return &EventSub{
		chain:  chain,
		filter: filter,
		bs:     NewBlockSub(chain, fromBlock),
	}
}

func (es *EventSub) Read() ([]*Event, []*Event, error) {
	nextBlks, removedBlks, err := es.bs.Read()
	if err != nil {
		return nil, nil, err
	}

	eventChanges, err := es.filterEvent(nextBlks)
	if err != nil {
		return nil, nil, err
	}

	eventRemoves, err := es.filterEvent(removedBlks)
	if err != nil {
		return nil, nil, err
	}

	return eventChanges, eventRemoves, nil
}

func (es *EventSub) filterEvent(blks []*block.Block) ([]*Event, error) {
	outputs, err := extractOutputs(es.chain, blks)
	if err != nil {
		return nil, err
	}

	result := []*Event{}
	for _, output := range outputs {
		for _, event := range output.Events {
			if es.filter.match(event) {
				result = append(result, newEvent(output.header, output.origin, output.tx, event))
			}
		}
	}

	return result, nil
}
