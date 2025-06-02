// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"errors"

	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

// FilterBuilder is the interface for event filtering.
type FilterBuilder interface {
	// InRange sets the range for event filtering.
	InRange(r *events.Range) FilterBuilder

	// WithOptions sets the filter options.
	WithOptions(opts *events.Options) FilterBuilder

	// OrderBy sets the order for event filtering.
	OrderBy(order logdb.Order) FilterBuilder

	// Execute performs the event filtering.
	Execute() ([]events.FilteredEvent, error)
}

// filterBuilder is the concrete implementation of FilterBuilder.
type filterBuilder struct {
	op      *operationBuilder
	evRange *events.Range
	opts    *events.Options
	order   logdb.Order
}

// InRange implements FilterBuilder.InRange.
func (b *filterBuilder) InRange(r *events.Range) FilterBuilder {
	b.evRange = r
	return b
}

// WithOptions implements FilterBuilder.WithOptions.
func (b *filterBuilder) WithOptions(opts *events.Options) FilterBuilder {
	b.opts = opts
	return b
}

// OrderBy implements FilterBuilder.OrderBy.
func (b *filterBuilder) OrderBy(order logdb.Order) FilterBuilder {
	b.order = order
	return b
}

// Execute implements FilterBuilder.Execute.
func (b *filterBuilder) Execute() ([]events.FilteredEvent, error) {
	event, ok := b.op.contract.abi.Events[b.op.method]
	if !ok {
		return nil, errors.New("event not found: " + b.op.method)
	}

	id := thor.Bytes32(event.Id())
	req := &events.EventFilter{
		Range:   b.evRange,
		Options: b.opts,
		Order:   b.order,
		CriteriaSet: []*events.EventCriteria{
			{
				Address: b.op.contract.addr,
				TopicSet: events.TopicSet{
					Topic0: &id,
				},
			},
		},
	}

	return b.op.contract.client.FilterEvents(req)
}
