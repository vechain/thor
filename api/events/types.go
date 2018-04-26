package events

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/api/transactions"
	"github.com/vechain/thor/eventdb"
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
	Range     *eventdb.Range
	Options   *eventdb.Options
	Order     eventdb.OrderType
}

func convertFilter(filter *Filter) *eventdb.Filter {
	f := &eventdb.Filter{
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
	Topics []*thor.Bytes32           `json:"topics"`
	Data   string                    `json:"data"`
	Block  transactions.BlockContext `json:"block"`
	Tx     transactions.TxContext    `json:"tx"`
}

//convert a eventdb.Event into a json format Event
func convertEvent(event *eventdb.Event) *FilteredEvent {
	fe := FilteredEvent{
		Data: hexutil.Encode(event.Data),
		Block: transactions.BlockContext{
			ID:        event.BlockID,
			Number:    event.BlockNumber,
			Timestamp: event.BlockTime,
		},
		Tx: transactions.TxContext{
			ID:     event.TxID,
			Origin: event.TxOrigin,
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
			block: (id     %v,
					num    %v,
					time   %v),
			tx:    (id     %v,
					origin %v)
			)`,
		e.Topics,
		e.Data,
		e.Block.ID,
		e.Block.Number,
		e.Block.Timestamp,
		e.Tx.ID,
		e.Tx.Origin,
	)
}
