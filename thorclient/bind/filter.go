// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package bind

import (
	"errors"

	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/thor"
)

type filterConfig struct {
	evRange *dto.Range
	opts    *dto.Options
	order   dto.Order
}

// FilterOption configures event filtering behavior.
type FilterOption func(*filterConfig)

// FilterBlocks filters events within the given block range.
func FilterBlocks(from, to uint64) FilterOption {
	return func(c *filterConfig) {
		c.evRange = &dto.Range{
			From: &from,
			To:   &to,
			Unit: dto.BlockRangeType,
		}
	}
}

// FilterTimestamps filters events within the given timestamp range.
func FilterTimestamps(from, to uint64) FilterOption {
	return func(c *filterConfig) {
		c.evRange = &dto.Range{
			From: &from,
			To:   &to,
			Unit: dto.TimeRangeType,
		}
	}
}

// FilterPagination sets pagination options for the filter.
func FilterPagination(offset, limit uint64) FilterOption {
	return func(c *filterConfig) {
		c.opts = &dto.Options{
			Offset: offset,
			Limit:  &limit,
		}
	}
}

// FilterOrder sets the sort order for returned events.
func FilterOrder(order dto.Order) FilterOption {
	return func(c *filterConfig) {
		c.order = order
	}
}

// FilterBuilder is the concrete implementation of FilterBuilder.
type FilterBuilder struct {
	op *MethodBuilder
}

// Execute runs the event filter with the given options.
func (b *FilterBuilder) Execute(options ...FilterOption) ([]dto.FilteredEvent, error) {
	event, ok := b.op.contract.abi.Events[b.op.method]
	if !ok {
		return nil, errors.New("event not found: " + b.op.method)
	}

	cfg := &filterConfig{}
	for _, opt := range options {
		opt(cfg)
	}
	if cfg.opts == nil {
		cfg.opts = &dto.Options{}
	}
	cfg.opts.IncludeIndexes = true

	id := thor.Bytes32(event.ID)
	req := &dto.EventFilter{
		Range:   cfg.evRange,
		Options: cfg.opts,
		Order:   cfg.order,
		CriteriaSet: []*dto.EventCriteria{
			{
				Address: b.op.contract.addr,
				TopicSet: dto.TopicSet{
					Topic0: &id,
				},
			},
		},
	}

	return b.op.contract.client.FilterEvents(req)
}
