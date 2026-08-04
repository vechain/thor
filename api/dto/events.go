// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package dto

import (
	"fmt"

	"github.com/vechain/thor/v2/thor"
)

// Order is the result ordering of log queries; wire-compatible with logdb.Order.
type Order string

const (
	ASC  Order = "asc"
	DESC Order = "desc"
)

// FilteredEvent only comes from one contract
type FilteredEvent struct {
	Address thor.Address    `json:"address"`
	Topics  []*thor.Bytes32 `json:"topics"`
	Data    string          `json:"data"`
	Meta    LogMeta         `json:"meta"`
}

type TopicSet struct {
	Topic0 *thor.Bytes32 `json:"topic0"`
	Topic1 *thor.Bytes32 `json:"topic1"`
	Topic2 *thor.Bytes32 `json:"topic2"`
	Topic3 *thor.Bytes32 `json:"topic3"`
	Topic4 *thor.Bytes32 `json:"topic4"`
}

type EventCriteria struct {
	Address *thor.Address `json:"address"`
	TopicSet
}

type Options struct {
	Offset         uint64  `json:"offset,omitempty"`
	Limit          *uint64 `json:"limit,omitempty"`
	IncludeIndexes bool    `json:"includeIndexes,omitempty"`
}

func (o *Options) Validate(maxLimit uint64, maxOffset uint64) error {
	if o == nil {
		return nil
	}
	if o.Limit != nil && *o.Limit > maxLimit {
		return fmt.Errorf("options.limit exceeds the maximum allowed value of %d", maxLimit)
	}

	if o.Offset > maxOffset {
		return fmt.Errorf("options.offset exceeds the maximum allowed value of %d", maxOffset)
	}

	return nil
}

type EventFilter struct {
	CriteriaSet []*EventCriteria `json:"criteriaSet,omitempty"`
	Range       *Range           `json:"range,omitempty"`
	Options     *Options         `json:"options,omitempty"`
	Order       Order            `json:"order,omitempty"`
}

type RangeType string

const (
	BlockRangeType RangeType = "block"
	TimeRangeType  RangeType = "time"
)

type Range struct {
	Unit RangeType `json:"unit,omitempty"`
	From *uint64   `json:"from,omitempty"`
	To   *uint64   `json:"to,omitempty"`
}

func (r *Range) Validate() error {
	if r == nil {
		return nil
	}
	if r.Unit != "" {
		if r.Unit != BlockRangeType && r.Unit != TimeRangeType {
			return fmt.Errorf("filter.Range.Unit must be either 'block' or 'time', got '%s'", r.Unit)
		}
	}

	if r.From == nil || r.To == nil {
		return nil // No range specified, which is valid
	}
	if *r.From > *r.To {
		return fmt.Errorf("filter.Range.To must be greater than or equal to filter.Range.From")
	}

	return nil
}
