// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/api/transactions"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
)

type TopicSet struct {
	Topic0 *thor.Bytes32 `json:"topic0"`
	Topic1 *thor.Bytes32 `json:"topic1"`
	Topic2 *thor.Bytes32 `json:"topic2"`
	Topic3 *thor.Bytes32 `json:"topic3"`
	Topic4 *thor.Bytes32 `json:"topic4"`
}

type Filter struct {
	Address   *thor.Address
	TopicSets []*TopicSet
	Range     *logdb.Range
	Options   *logdb.Options
	Order     logdb.Order
}

func convertFilter(filter *Filter) *logdb.EventFilter {
	f := &logdb.EventFilter{
		Address: filter.Address,
		Range:   filter.Range,
		Options: filter.Options,
		Order:   filter.Order,
	}
	if len(filter.TopicSets) > 0 {
		var topicSets [][5]*thor.Bytes32
		for _, topicSet := range filter.TopicSets {
			var topics [5]*thor.Bytes32
			topics[0] = topicSet.Topic0
			topics[1] = topicSet.Topic1
			topics[2] = topicSet.Topic2
			topics[3] = topicSet.Topic3
			topics[4] = topicSet.Topic4
			topicSets = append(topicSets, topics)
		}
		f.TopicSet = topicSets
	}
	return f
}

// FilteredEvent only comes from one contract
type FilteredEvent struct {
	Topics []*thor.Bytes32      `json:"topics"`
	Data   string               `json:"data"`
	Meta   transactions.LogMeta `json:"meta"`
}

//convert a logdb.Event into a json format Event
func convertEvent(event *logdb.Event) *FilteredEvent {
	fe := FilteredEvent{
		Data: hexutil.Encode(event.Data),
		Meta: transactions.LogMeta{
			BlockID:        event.BlockID,
			BlockNumber:    event.BlockNumber,
			BlockTimestamp: event.BlockTime,
			TxID:           event.TxID,
			TxOrigin:       event.TxOrigin,
		},
	}
	fe.Topics = make([]*thor.Bytes32, 0)
	for i := 0; i < 5; i++ {
		if event.Topics[i] != nil {
			fe.Topics = append(fe.Topics, event.Topics[i])
		}
	}
	return &fe
}

func (e *FilteredEvent) String() string {
	return fmt.Sprintf(`
		Event(
			topics:        %v,
			data:          %v,
			meta: (blockID     %v,
				blockNumber    %v,
				blockTimestamp %v),
				txID     %v,
				txOrigin %v)
			)`,
		e.Topics,
		e.Data,
		e.Meta.BlockID,
		e.Meta.BlockNumber,
		e.Meta.BlockTimestamp,
		e.Meta.TxID,
		e.Meta.TxOrigin,
	)
}
