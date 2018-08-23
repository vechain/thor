package subscriptions

import (
	"context"

	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
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

func (es *EventSub) FromBlock() thor.Bytes32 { return es.filter.FromBlock }

func (es *EventSub) UpdateFilter(bestID thor.Bytes32) {
	es.filter.FromBlock = bestID
}

// from open, to closed
func (es *EventSub) SliceChain(from, to thor.Bytes32) ([]interface{}, error) {
	return sliceChain(from, to, es.chain, makeAnalyse(es.filterEvent))
}

func (es *EventSub) filterEvent(output *tx.Output) []interface{} {
	// TODO
	return nil
}

func (es *EventSub) Read(ctx context.Context) (tx.Events, tx.Events, error) {
	changes, removes, err := read(ctx, es.ch, es.chain, es)
	if err != nil {
		return nil, nil, err
	}

	convertEvent := func(slice []interface{}) tx.Events {
		result := tx.Events{}
		for _, v := range slice {
			if events, ok := v.(tx.Events); ok {
				for _, event := range events {
					result = append(result, event)
				}
			}
		}
		return result
	}

	return convertEvent(changes), convertEvent(removes), err
}
