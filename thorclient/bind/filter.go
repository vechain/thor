// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

// FilterBuilder is the concrete implementation of FilterBuilder.
type FilterBuilder struct {
	op      *MethodBuilder
	evRange *api.Range
	opts    *api.Options
	order   logdb.Order
}

// InRange implements FilterBuilder.InRange.
func (b *FilterBuilder) InRange(r *api.Range) *FilterBuilder {
	b.evRange = r
	return b
}

// WithOptions implements FilterBuilder.WithOptions.
func (b *FilterBuilder) WithOptions(opts *api.Options) *FilterBuilder {
	b.opts = opts
	return b
}

// OrderBy implements FilterBuilder.OrderBy.
func (b *FilterBuilder) OrderBy(order logdb.Order) *FilterBuilder {
	b.order = order
	return b
}

// Execute implements FilterBuilder.Execute.
func (b *FilterBuilder) Execute() ([]api.FilteredEvent, error) {
	event, ok := b.op.contract.abi.Events[b.op.method]
	if !ok {
		return nil, errors.New("event not found: " + b.op.method)
	}

	id := thor.Bytes32(event.Id())
	req := &api.EventFilter{
		Range:   b.evRange,
		Options: b.opts,
		Order:   b.order,
		CriteriaSet: []*api.EventCriteria{
			{
				Address: b.op.contract.addr,
				TopicSet: api.TopicSet{
					Topic0: &id,
				},
			},
		},
	}

	return b.op.contract.client.FilterEvents(req)
}
