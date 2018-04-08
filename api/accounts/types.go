package accounts

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
)

//Account for marshal account
type Account struct {
	Balance math.HexOrDecimal256 `json:"balance,string"`
	Energy  math.HexOrDecimal256 `json:"energy,string"`
	HasCode bool                 `json:"hasCode"`
}

//ContractCall represents contract-call body
type ContractCall struct {
	Value    *math.HexOrDecimal256 `json:"value,string"`
	Data     string                `json:"data"`
	Gas      uint64                `json:"gas"`
	GasPrice *math.HexOrDecimal256 `json:"gasPrice,string"`
	Caller   thor.Address          `json:"caller"`
}

type VMOutput struct {
	Data     string `json:"data,string"`
	GasUsed  uint64 `json:"gas"`
	Reverted bool   `json:"reverted"`
	VMError  string `json:"vmError"`
}

func convertVMOutputWithInputGas(vo *vm.Output, inputGas uint64) *VMOutput {
	gasUsed := inputGas - vo.LeftOverGas
	var (
		vmError  string
		reverted bool
	)

	if vo.VMErr != nil {
		reverted = true
		vmError = vo.VMErr.Error()
	}

	return &VMOutput{
		Data:     hexutil.Encode(vo.Value),
		GasUsed:  gasUsed,
		Reverted: reverted,
		VMError:  vmError,
	}
}

type TopicSet struct {
	Topic0 *thor.Bytes32 `json:"topic0"`
	Topic1 *thor.Bytes32 `json:"topic1"`
	Topic2 *thor.Bytes32 `json:"topic2"`
	Topic3 *thor.Bytes32 `json:"topic3"`
	Topic4 *thor.Bytes32 `json:"topic4"`
}

type LogFilter struct {
	Address   *thor.Address `json:"address"` // always a contract address
	TopicSets []*TopicSet   `json:"topicSets"`
	Range     *logdb.Range
	Options   *logdb.Options
}

func convertLogFilter(logFilter *LogFilter) *logdb.LogFilter {
	f := &logdb.LogFilter{
		Address: logFilter.Address,
		Range:   logFilter.Range,
		Options: logFilter.Options,
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
	BlockID     thor.Bytes32    `json:"blockID"`
	BlockNumber uint32          `json:"fromBlock"`
	BlockTime   uint64          `json:"blockTime"`
	LogIndex    uint32          `json:"logIndex"`
	TxID        thor.Bytes32    `json:"txID"`
	TxOrigin    thor.Address    `json:"txOrigin"` //contract caller
	Data        string          `json:"data"`
	Topics      []*thor.Bytes32 `json:"topics"`
}

//convert a logdb.Log into a json format log
func convertLog(log *logdb.Log) FilteredLog {
	l := FilteredLog{
		BlockID:     log.BlockID,
		BlockNumber: log.BlockNumber,
		LogIndex:    log.LogIndex,
		TxID:        log.TxID,
		TxOrigin:    log.TxOrigin,
		Data:        hexutil.Encode(log.Data),
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
			blockID:     %v,
			blockNumber: %v,
			txID:        %v,
			txOrigin:    %v,
			data:        %v,
			topics:      %v)`, log.BlockID,
		log.BlockNumber,
		log.TxID,
		log.TxOrigin,
		log.Data,
		log.Topics)
}
