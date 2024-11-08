// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events

import (
	"math"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

type LogMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
	TxID           thor.Bytes32 `json:"txID"`
	TxOrigin       thor.Address `json:"txOrigin"`
	ClauseIndex    uint32       `json:"clauseIndex"`
	TxIndex        *uint32      `json:"txIndex,omitempty"`
	LogIndex       *uint32      `json:"logIndex,omitempty"`
}

type TopicSet struct {
	Topic0 *thor.Bytes32 `json:"topic0"`
	Topic1 *thor.Bytes32 `json:"topic1"`
	Topic2 *thor.Bytes32 `json:"topic2"`
	Topic3 *thor.Bytes32 `json:"topic3"`
	Topic4 *thor.Bytes32 `json:"topic4"`
}

// FilteredEvent only comes from one contract
type FilteredEvent struct {
	Address thor.Address    `json:"address"`
	Topics  []*thor.Bytes32 `json:"topics"`
	Data    string          `json:"data"`
	Meta    LogMeta         `json:"meta"`
}

// convert a logdb.Event into a json format Event
func convertEvent(event *logdb.Event, addIndexes bool) *FilteredEvent {
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
	for i := 0; i < 5; i++ {
		if event.Topics[i] != nil {
			fe.Topics = append(fe.Topics, event.Topics[i])
		}
	}
	return fe
}

type EventCriteria struct {
	Address *thor.Address `json:"address"`
	TopicSet
}

type Options struct {
	Offset         uint64
	Limit          uint64
	IncludeIndexes bool
}

type EventFilter struct {
	CriteriaSet []*EventCriteria
	Range       *Range
	Options     *Options
	Order       logdb.Order // default asc
}

func convertEventFilter(chain *chain.Chain, filter *EventFilter) (*logdb.EventFilter, error) {
	rng, err := ConvertRange(chain, filter.Range)
	if err != nil {
		return nil, err
	}
	f := &logdb.EventFilter{
		Range: rng,
		Options: &logdb.Options{
			Offset: filter.Options.Offset,
			Limit:  filter.Options.Limit,
		},
		Order: filter.Order,
	}
	if len(filter.CriteriaSet) > 0 {
		f.CriteriaSet = make([]*logdb.EventCriteria, len(filter.CriteriaSet))
		for i, criterion := range filter.CriteriaSet {
			var topics [5]*thor.Bytes32
			topics[0] = criterion.Topic0
			topics[1] = criterion.Topic1
			topics[2] = criterion.Topic2
			topics[3] = criterion.Topic3
			topics[4] = criterion.Topic4
			f.CriteriaSet[i] = &logdb.EventCriteria{
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
	Unit RangeType
	From uint64
	To   uint64
}

func ConvertRange(chain *chain.Chain, r *Range) (*logdb.Range, error) {
	if r == nil {
		return nil, nil
	}
	if r.Unit == TimeRangeType {
		emptyRange := logdb.Range{
			From: math.MaxUint32,
			To:   math.MaxUint32,
		}

		genesis, err := chain.GetBlockHeader(0)
		if err != nil {
			return nil, err
		}
		if r.To < genesis.Timestamp() {
			return &emptyRange, nil
		}
		head, err := chain.GetBlockHeader(block.Number(chain.HeadID()))
		if err != nil {
			return nil, err
		}
		if r.From > head.Timestamp() {
			return &emptyRange, nil
		}

		fromHeader, err := chain.FindBlockHeaderByTimestamp(r.From, 1)
		if err != nil {
			return nil, err
		}

		toHeader, err := chain.FindBlockHeaderByTimestamp(r.To, -1)
		if err != nil {
			return nil, err
		}

		return &logdb.Range{
			From: fromHeader.Number(),
			To:   toHeader.Number(),
		}, nil
	}
	return &logdb.Range{
		From: uint32(r.From),
		To:   uint32(r.To),
	}, nil
}
