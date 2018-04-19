package logdb

import (
	"fmt"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//Log format tx.Log to store in db
type Log struct {
	BlockID     thor.Bytes32
	LogIndex    uint32
	BlockNumber uint32
	BlockTime   uint64
	TxID        thor.Bytes32
	TxOrigin    thor.Address //contract caller
	Address     thor.Address // always a contract address
	Topics      [5]*thor.Bytes32
	Data        []byte
}

//NewLog return a format log
func NewLog(header *block.Header, logIndex uint32, txID thor.Bytes32, txOrigin thor.Address, txLog *tx.Log) *Log {
	l := &Log{
		BlockID:     header.ID(),
		LogIndex:    logIndex,
		BlockNumber: header.Number(),
		BlockTime:   header.Timestamp(),
		TxID:        txID,
		TxOrigin:    txOrigin,
		Address:     txLog.Address, // always a contract address
		Data:        txLog.Data,
	}
	for i, topic := range txLog.Topics {
		// variable topic from range shares the same address, clone topic when need to use topic's pointer
		topic := topic
		l.Topics[i] = &topic
	}
	return l
}

func (log *Log) String() string {
	return fmt.Sprintf(`
		Log(
			blockID:     %v,
			logIndex:	 %v,
			blockNumber: %v,
			blockTime:   %v,
			txID:        %v,
			txOrigin:    %v,
			address:     %v,
			topic0:      %v,
			topic1:      %v,
			topic2:      %v,
			topic3:      %v,
			topic4:      %v,
			data:        0x%x)`,
		log.BlockID,
		log.LogIndex,
		log.BlockNumber,
		log.BlockTime,
		log.TxID,
		log.TxOrigin,
		log.Address,
		log.Topics[0],
		log.Topics[1],
		log.Topics[2],
		log.Topics[3],
		log.Topics[4],
		log.Data)
}
