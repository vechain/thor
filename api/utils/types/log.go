package types

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
)

// Log for json marshal
type Log struct {
	BlockID     thor.Hash     `json:"blockID"`
	BlockNumber uint32        `json:"fromBlock"`
	LogIndex    uint32        `json:"logIndex"`
	TxID        thor.Hash     `json:"txID"`
	TxOrigin    thor.Address  `json:"txOrigin"` //contract caller
	Address     thor.Address  `json:"address"`  // always a contract address
	Data        string        `json:"data"`
	Topics      [5]*thor.Hash `json:"topics"`
}

//ConvertLog convert a logdb.Log into a json format log
func ConvertLog(log *logdb.Log) Log {
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
