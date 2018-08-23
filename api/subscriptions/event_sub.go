package subscriptions

import (
	"context"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/tx"
)

type EventSub struct {
	ch     chan struct{} // When chain changed, this channel will be readable
	chain  *chain.Chain
	filter *EventFilter
}

func NewEventSub(ch chan struct{}, chain *chain.Chain, filter *EventFilter) *EventSub {
	return &EventSub{
		ch:     ch,
		chain:  chain,
		filter: filter,
	}
}

// func (es *EventSub) FromBlock() thor.Bytes32 { return es.filter.FromBlock }

// func (es *EventSub) UpdateFilter(bestID thor.Bytes32) {
// 	es.filter.FromBlock = bestID
// }

// // from open, to closed
// func (es *EventSub) SliceChain(from, to thor.Bytes32) ([]interface{}, error) {
// 	return sliceChain(from, to, es.chain, makeAnalyse(es.filterEvent))
// }

// func (es *EventSub) filterEvent(output *tx.Output) []interface{} {
// 	// TODO
// 	return nil
// }

func (es *EventSub) Read(ctx context.Context) (tx.Events, tx.Events, error) {
	bs := NewBlockSub(es.ch, es.chain, es.filter.FromBlock)
	blkChanges, blkRemoves, err := bs.Read(ctx)
	if err != nil {
		return nil, nil, err
	}
	es.filter.FromBlock = bs.fromBlock

	eventChanges, err := es.filterEvent(blkChanges)
	if err != nil {
		return nil, nil, err
	}

	eventRemoves, err := es.filterEvent(blkRemoves)
	if err != nil {
		return nil, nil, err
	}

	return eventChanges, eventRemoves, nil
}

func (es *EventSub) filterEvent(blks []*block.Block) (tx.Events, error) {
	outputs, err := outputs(es.chain, blks)
	if err != nil {
		return nil, err
	}

	result := tx.Events{}
	for _, output := range outputs {
		for _, event := range output.Events {
			result = append(result, event)
		}
	}
	return result, nil
}
