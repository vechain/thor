package accounts

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
)

//Account for marshal account
type Account struct {
	Balance math.HexOrDecimal256 `json:"balance,string"`
	Code    string               `json:"code"`
}

//ContractCallBody represents contract-call body
type ContractCallBody struct {
	Input   string              `json:"input"`
	Options ContractCallOptions `json:"options"`
}

// ContractCallOptions represents options in contract-call body
type ContractCallOptions struct {
	ClauseIndex uint32                `json:"clauseIndex"`
	Gas         uint64                `json:"gas,string"`
	From        *thor.Address         `json:"from"`
	GasPrice    *math.HexOrDecimal256 `json:"gasPrice,string"`
	TxID        *thor.Bytes32         `json:"txID"`
	Value       *math.HexOrDecimal256 `json:"value,string"`
}

type FilterTopics struct {
	TopicSet [][5]*thor.Bytes32 `json:"topicSet"`
}

// Log for json marshal
type Log struct {
	BlockID     thor.Bytes32     `json:"blockID"`
	BlockNumber uint32           `json:"fromBlock"`
	LogIndex    uint32           `json:"logIndex"`
	TxID        thor.Bytes32     `json:"txID"`
	TxOrigin    thor.Address     `json:"txOrigin"` //contract caller
	Address     thor.Address     `json:"address"`  // always a contract address
	Data        string           `json:"data"`
	Topics      [5]*thor.Bytes32 `json:"topics"`
}

//convert a logdb.Log into a json format log
func convertLog(log *logdb.Log) Log {
	l := Log{
		BlockID:     log.BlockID,
		BlockNumber: log.BlockNumber,
		LogIndex:    log.LogIndex,
		TxID:        log.TxID,
		TxOrigin:    log.TxOrigin,
		Address:     log.Address,
		Data:        hexutil.Encode(log.Data),
	}
	for i := 0; i < 5; i++ {
		if log.Topics[i] != nil {
			l.Topics[i] = log.Topics[i]
		}
	}
	return l
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
