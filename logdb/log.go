package logdb

import (
	"encoding/hex"
	"fmt"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//DBLog format with raw type
type DBLog struct {
	blockID     string
	blockNumber uint32
	logIndex    uint32
	txID        string
	txOrigin    string
	address     string
	data        string
	topic0      string
	topic1      string
	topic2      string
	topic3      string
	topic4      string
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
	bid, err := thor.ParseHash(dbLog.blockID)
	if err != nil {
		return nil, err
	}
	txid, err := thor.ParseHash(dbLog.txID)
	if err != nil {
		return nil, err
	}
	txori, err := thor.ParseAddress(dbLog.txOrigin)
	if err != nil {
		return nil, err
	}
	addr, err := thor.ParseAddress(dbLog.address)
	if err != nil {
		return nil, err
	}
	data, err := hex.DecodeString(dbLog.data)
	if err != nil {
		return nil, err
	}
	l := &Log{
		BlockID:     bid,
		BlockNumber: dbLog.blockNumber,
		LogIndex:    dbLog.logIndex,
		TxID:        txid,
		TxOrigin:    txori,
		Address:     addr, // always a contract address
		Data:        data,
	}
	if dbLog.topic0 != "NULL" {
		t0, err := thor.ParseHash(dbLog.topic0)
		if err != nil {
			return nil, err
		}
		l.Topics[0] = &t0
	}
	if dbLog.topic1 != "NULL" {
		t1, err := thor.ParseHash(dbLog.topic1)
		if err != nil {
			return nil, err
		}
		l.Topics[1] = &t1
	}
	if dbLog.topic2 != "NULL" {
		t2, err := thor.ParseHash(dbLog.topic2)
		if err != nil {
			return nil, err
		}
		l.Topics[2] = &t2
	}
	if dbLog.topic3 != "NULL" {
		t3, err := thor.ParseHash(dbLog.topic3)
		if err != nil {
			return nil, err
		}
		l.Topics[3] = &t3
	}
	if dbLog.topic4 != "NULL" {
		t4, err := thor.ParseHash(dbLog.topic4)
		if err != nil {
			return nil, err
		}
		l.Topics[4] = &t4
	}
	return l, nil

}

func formatHash(value *thor.Hash) interface{} {
	if value == nil {
		return "NULL"
	}
	return value
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
		formatHash(log.Topics[0]),
		formatHash(log.Topics[1]),
		formatHash(log.Topics[2]),
		formatHash(log.Topics[3]),
		formatHash(log.Topics[4]))
}
