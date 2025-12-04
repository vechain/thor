// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
)

// FilteredEvent only comes from one contract
type FilteredEvent struct {
	Address thor.Address    `json:"address"`
	Topics  []*thor.Bytes32 `json:"topics"`
	Data    string          `json:"data"`
	Meta    LogMeta         `json:"meta"`
}

// Convert a logsdb.Event into a json format Event
func ConvertEvent(event *logsdb.Event, addIndexes bool) *FilteredEvent {
	fe := &FilteredEvent{
		Address: event.Address,
		Data:    hexutil.Encode(event.Data),
		Meta: LogMeta{
			BlockID:        event.BlockID,
			BlockNumber:    event.BlockNumber,
			BlockTimestamp: event.BlockTime,
			TxID:           event.TxID,
			TxOrigin:       event.TxOrigin,
			ClauseIndex:    event.ClauseIndex,
		},
	}

	if addIndexes {
		fe.Meta.TxIndex = &event.TxIndex
		fe.Meta.LogIndex = &event.LogIndex
	}

	fe.Topics = make([]*thor.Bytes32, 0)
	for i := range 5 {
		if event.Topics[i] != nil {
			fe.Topics = append(fe.Topics, event.Topics[i])
		}
	}
	return fe
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

func (o *Options) Validate(limit uint64) error {
	if o == nil {
		return nil
	}
	if o.Limit != nil && *o.Limit > limit {
		return fmt.Errorf("options.limit exceeds the maximum allowed value of %d", limit)
	}
	if o.Offset > math.MaxInt64 {
		return fmt.Errorf("options.offset exceeds the maximum allowed value of %d", math.MaxInt64)
	}

	return nil
}

type EventFilter struct {
	CriteriaSet []*EventCriteria `json:"criteriaSet,omitempty"`
	Range       *Range           `json:"range,omitempty"`
	Options     *Options         `json:"options,omitempty"`
	Order       logsdb.Order     `json:"order,omitempty"`
}

func ConvertEventFilter(chain *chain.Chain, filter *EventFilter) (*logsdb.EventFilter, error) {
	rng, err := ConvertRange(chain, filter.Range)
	if err != nil {
		return nil, err
	}
	f := &logsdb.EventFilter{
		Range: rng,
		Options: &logsdb.Options{
			Offset: filter.Options.Offset,
			// validated or default value set at the API level
			Limit: *filter.Options.Limit,
		},
		Order: filter.Order,
	}
	if len(filter.CriteriaSet) > 0 {
		f.CriteriaSet = make([]*logsdb.EventCriteria, len(filter.CriteriaSet))
		for i, criterion := range filter.CriteriaSet {
			var topics [5]*thor.Bytes32
			topics[0] = criterion.Topic0
			topics[1] = criterion.Topic1
			topics[2] = criterion.Topic2
			topics[3] = criterion.Topic3
			topics[4] = criterion.Topic4
			f.CriteriaSet[i] = &logsdb.EventCriteria{
				Address: criterion.Address,
				Topics:  topics,
			}
		}
	}
	return f, nil
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

var emptyRange = logsdb.Range{
	From: logsdb.MaxBlockNumber,
	To:   logsdb.MaxBlockNumber,
}

func ConvertRange(chain *chain.Chain, r *Range) (*logsdb.Range, error) {
	if r == nil {
		return nil, nil
	}

	if r.Unit == TimeRangeType {
		genesis, err := chain.GetBlockHeader(0)
		if err != nil {
			return nil, err
		}
		if r.To != nil && *r.To < genesis.Timestamp() {
			return &emptyRange, nil
		}
		head, err := chain.GetBlockHeader(block.Number(chain.HeadID()))
		if err != nil {
			return nil, err
		}
		if r.From != nil && *r.From > head.Timestamp() {
			return &emptyRange, nil
		}

		fromHeader := genesis
		if r.From != nil {
			fromHeader, err = chain.FindBlockHeaderByTimestamp(*r.From, 1)
			if err != nil {
				return nil, err
			}
		}

		toHeader := head
		if r.To != nil {
			toHeader, err = chain.FindBlockHeaderByTimestamp(*r.To, -1)
			if err != nil {
				return nil, err
			}
		}

		return &logsdb.Range{
			From: fromHeader.Number(),
			To:   toHeader.Number(),
		}, nil
	}

	// Units are block numbers - numbers will have a max ceiling at logdb.MaxBlockNumber
	if r.From != nil && *r.From > logsdb.MaxBlockNumber {
		return &emptyRange, nil
	}

	from := uint32(0)
	if r.From != nil {
		from = uint32(*r.From)
	}

	to := uint32(logsdb.MaxBlockNumber)
	if r.To != nil && *r.To < logsdb.MaxBlockNumber {
		to = uint32(*r.To)
	}

	return &logsdb.Range{
		From: from,
		To:   to,
	}, nil
}
