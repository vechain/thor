package subscriptions

import (
	"context"

	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/logdb"
)

type EventSub struct {
	ch     chan struct{} // When chain changed, this channel will be readable
	chain  *chain.Chain
	filter *FilterQuery
}

func NewEventSub(ch chan struct{}, chain *chain.Chain, filter *FilterQuery) *EventSub {
	return &EventSub{
		ch:     ch,
		chain:  chain,
		filter: filter,
	}
}

func (bs *EventSub) Read(ctx context.Context) ([]*logdb.Event, []*logdb.Event, error) {
	return nil, nil, ctx.Err()
}
