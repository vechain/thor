package logdb

import (
	"fmt"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//DBLog format with raw type
type DBLog struct {
	blockID     []byte
	blockNumber uint32
	logIndex    uint32
	txID        []byte
	txOrigin    []byte
	address     []byte
	data        []byte
	topic0      []byte
	topic1      []byte
	topic2      []byte
	topic3      []byte
	topic4      []byte
}

//Log format tx.Log to store in db
type Log struct {
	BlockID     thor.Hash     `json:"blockID,string"`
	BlockNumber uint32        `json:"fromBlock"`
	LogIndex    uint32        `json:"logIndex"`
	TxID        thor.Hash     `json:"txID,string"`
	TxOrigin    thor.Address  `json:"txOrigin,string"` //contract caller
	Address     thor.Address  `json:"address,string"`  // always a contract address
	Data        []byte        `json:"data,string"`
	Topics      [5]*thor.Hash `json:"topics,string"`
}

//NewLog return a format log
func NewLog(blockID thor.Hash, blockNumber uint32, logIndex uint32, txID thor.Hash, txOrigin thor.Address, txLog *tx.Log) *Log {
	l := &Log{
		BlockID:     blockID,
		BlockNumber: blockNumber,
		LogIndex:    logIndex,
		TxID:        txID,
		TxOrigin:    txOrigin,
		Address:     txLog.Address, // always a contract address
		Data:        txLog.Data,
	}
	for i := 0; i < len(txLog.Topics); i++ {
		l.Topics[i] = &txLog.Topics[i]
	}
	return l
}

func (dbLog *DBLog) toLog() (*Log, error) {
	l := &Log{
		BlockID:     thor.BytesToHash(dbLog.blockID),
		BlockNumber: dbLog.blockNumber,
		LogIndex:    dbLog.logIndex,
		TxID:        thor.BytesToHash(dbLog.txID),
		TxOrigin:    thor.BytesToAddress(dbLog.txOrigin),
		Address:     thor.BytesToAddress(dbLog.address), // always a contract address
		Data:        dbLog.data,
	}
	if dbLog.topic0 != nil {
		t0 := thor.BytesToHash(dbLog.topic0)
		l.Topics[0] = &t0
	}
	if dbLog.topic1 != nil {
		t1 := thor.BytesToHash(dbLog.topic1)
		l.Topics[1] = &t1
	}
	if dbLog.topic2 != nil {
		t2 := thor.BytesToHash(dbLog.topic2)
		l.Topics[2] = &t2
	}
	if dbLog.topic3 != nil {
		t3 := thor.BytesToHash(dbLog.topic3)
		l.Topics[3] = &t3
	}
	if dbLog.topic4 != nil {
		t4 := thor.BytesToHash(dbLog.topic4)
		l.Topics[4] = &t4
	}
	return l, nil

}

func formatHash(value *thor.Hash) interface{} {
	if value == nil {
		return nil
	}
	return value.Bytes()
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
			topic4:      %v)`, log.BlockID.String(),
		log.BlockNumber,
		log.TxID.String(),
		log.TxOrigin.String(),
		log.Address.String(),
		[]byte(log.Data),
		log.Topics[0],
		log.Topics[1],
		log.Topics[2],
		log.Topics[3],
		log.Topics[4])
}

func (dbLog *DBLog) String() string {
	return fmt.Sprintf(`
		DBLog(
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
			topic4:      %v)`, dbLog.blockID,
		dbLog.blockNumber,
		dbLog.txID,
		dbLog.txOrigin,
		dbLog.address,
		dbLog.data,
		dbLog.topic0,
		dbLog.topic1,
		dbLog.topic2,
		dbLog.topic3,
		dbLog.topic4)
}
