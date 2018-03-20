package types

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
)

// Log for json marshal
type Log struct {
	BlockID     string    `json:"blockID"`
	BlockNumber uint32    `json:"fromBlock"`
	LogIndex    uint32    `json:"logIndex"`
	TxID        string    `json:"txID"`
	TxOrigin    string    `json:"txOrigin"` //contract caller
	Address     string    `json:"address"`  // always a contract address
	Data        string    `json:"data"`
	Topics      [5]string `json:"topics"`
}

//FilterOption option filter
type FilterOption struct {
	FromBlock uint32      `json:"fromBlock"`
	ToBlock   uint32      `json:"toBlock"`
	Address   string      `json:"address"` // always a contract address
	TopicSet  [][5]string `json:"topicSet"`
}

//ConvertLog convert a logdb.Log into a json format log
func ConvertLog(log *logdb.Log) Log {
	l := Log{
		BlockID:     log.BlockID.String(),
		BlockNumber: log.BlockNumber,
		LogIndex:    log.LogIndex,
		TxID:        log.TxID.String(),
		TxOrigin:    log.TxOrigin.String(),
		Address:     log.Address.String(),
		Data:        hexutil.Encode(log.Data),
	}
	for i := 0; i < 5; i++ {
		if log.Topics[i] != nil {
			l.Topics[i] = log.Topics[i].String()
		}
	}
	return l
}

//ToLogFilter convert a FilterOption to logdb.FilterOption
func (filter *FilterOption) ToLogFilter() (*logdb.FilterOption, error) {
	op := &logdb.FilterOption{
		FromBlock: filter.FromBlock,
		ToBlock:   filter.ToBlock,
	}
	if filter.Address != "" {
		addr, err := thor.ParseAddress(filter.Address)
		if err != nil {
			return nil, err
		}
		op.Address = &addr
	}
	if len(filter.TopicSet) > 0 {
		op.TopicSet = make([][5]*thor.Hash, len(filter.TopicSet))
		for i, topics := range filter.TopicSet {
			for j, topic := range topics {
				if topic != "" {
					t, err := thor.ParseHash(filter.TopicSet[i][j])
					if err != nil {
						return nil, err
					}
					op.TopicSet[i][j] = &t
				}
			}
		}

	}

	return op, nil
}

func (log *Log) String() string {
	return fmt.Sprintf(`
		Log(
			blockID:     %v,
			blockNumber: %v,
			txID:        %v,
			txOrigin:    %v,
			address:     %v,
			data:        %v,
			topic0:      %v,
			topic1:      %v,
			topic2:      %v,
			topic3:      %v,
			topic4:      %v)`, log.BlockID,
		log.BlockNumber,
		log.TxID,
		log.TxOrigin,
		log.Address,
		log.Data,
		log.Topics[0],
		log.Topics[1],
		log.Topics[2],
		log.Topics[3],
		log.Topics[4])
}
