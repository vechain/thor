package logs

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

type LogFilter struct {
	Address   *thor.Address
	TopicSets []*TopicSet
	Range     *logdb.Range
	Options   *logdb.Options
	Order     logdb.OrderType
}

func convertLogFilter(logFilter *LogFilter) *logdb.LogFilter {
	f := &logdb.LogFilter{
		Address: logFilter.Address,
		Range:   logFilter.Range,
		Options: logFilter.Options,
		Order:   logFilter.Order,
	}
	if len(logFilter.TopicSets) > 0 {
		var topicSets [][5]*thor.Bytes32
		for _, topicSet := range logFilter.TopicSets {
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

// FilteredLog only comes from one contract
type FilteredLog struct {
	Topics []*thor.Bytes32           `json:"topics"`
	Data   string                    `json:"data"`
	Block  transactions.BlockContext `json:"block"`
	Tx     transactions.TxContext    `json:"tx"`
}

//convert a logdb.Log into a json format log
func convertLog(log *logdb.Log) FilteredLog {
	l := FilteredLog{
		Data: hexutil.Encode(log.Data),
		Block: transactions.BlockContext{
			ID:        log.BlockID,
			Number:    log.BlockNumber,
			Timestamp: log.BlockTime,
		},
		Tx: transactions.TxContext{
			ID:     log.TxID,
			Origin: log.TxOrigin,
		},
	}
	l.Topics = make([]*thor.Bytes32, 0)
	for i := 0; i < 5; i++ {
		if log.Topics[i] != nil {
			l.Topics = append(l.Topics, log.Topics[i])
		}
	}
	return l
}

func (log *FilteredLog) String() string {
	return fmt.Sprintf(`
		Log(
			topics:        %v,
			data:          %v,
			block: (id     %v,
					num    %v,
					time   %v),
			tx:    (id     %v,
					origin %v)
			)`,
		log.Topics,
		log.Data,
		log.Block.ID,
		log.Block.Number,
		log.Block.Timestamp,
		log.Tx.ID,
		log.Tx.Origin,
	)
}
