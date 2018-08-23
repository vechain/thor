package subscriptions

import (
	"context"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
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

func (es *EventSub) Read(ctx context.Context) ([]*Event, []*Event, error) {
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

func (es *EventSub) filterEvent(blks []*block.Block) ([]*Event, error) {
	result := []*Event{}
	for _, blk := range blks {
		receipts, err := es.chain.GetBlockReceipts(blk.Header().ID())
		if err != nil {
			return nil, err
		}

		for i, receipt := range receipts {
			for _, output := range receipt.Outputs {
				for _, event := range output.Events {
					if es.filter.match(event) {
						v, err := newEvent(blk.Header(), blk.Transactions()[i], event)
						if err != nil {
							return nil, err
						}
						result = append(result, v)
					}
				}
			}
		}
	}
	return result, nil
}
