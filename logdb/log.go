package logdb

import (
	"fmt"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//DBLog format with raw type
type DBLog struct {
	blockID     string
	blockNumber uint32
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
	blockID     thor.Hash
	blockNumber uint32
	txID        thor.Hash
	txOrigin    thor.Address //contract caller
	address     thor.Address // always a contract address
	data        []byte
	topic0      *thor.Hash
	topic1      *thor.Hash
	topic2      *thor.Hash
	topic3      *thor.Hash
	topic4      *thor.Hash
}

//NewLog return a format log
func NewLog(blockID thor.Hash, blockNumber uint32, txID thor.Hash, txOrigin thor.Address, txLog *tx.Log) *Log {
	l := &Log{
		blockID:     blockID,
		blockNumber: blockNumber,
		txID:        txID,
		txOrigin:    txOrigin,
		address:     txLog.Address, // always a contract address
		data:        txLog.Data,
		topic0:      nil,
		topic1:      nil,
		topic2:      nil,
		topic3:      nil,
		topic4:      nil,
	}
	for i := 0; i < len(txLog.Topics); i++ {
		switch i {
		case 0:
			l.topic0 = &txLog.Topics[0]
			break
		case 1:
			l.topic1 = &txLog.Topics[1]
			break
		case 2:
			l.topic2 = &txLog.Topics[2]
			break
		case 3:
			l.topic3 = &txLog.Topics[3]
			break
		case 4:
			l.topic4 = &txLog.Topics[4]
			break
		}
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
	l := &Log{
		blockID:     bid,
		blockNumber: dbLog.blockNumber,
		txID:        txid,
		txOrigin:    txori,
		address:     addr, // always a contract address
		data:        []byte(dbLog.data),
		topic0:      nil,
		topic1:      nil,
		topic2:      nil,
		topic3:      nil,
		topic4:      nil,
	}
	if dbLog.topic0 == "NULL" {
		l.topic0 = nil
	} else {
		t0, err := thor.ParseHash(dbLog.topic0)
		if err != nil {
			return nil, err
		}
		l.topic0 = &t0
	}
	if dbLog.topic1 == "NULL" {
		l.topic1 = nil
	} else {
		t1, err := thor.ParseHash(dbLog.topic1)
		if err != nil {
			return nil, err
		}
		l.topic1 = &t1
	}
	if dbLog.topic2 == "NULL" {
		l.topic2 = nil
	} else {
		t2, err := thor.ParseHash(dbLog.topic2)
		if err != nil {
			return nil, err
		}
		l.topic2 = &t2
	}
	if dbLog.topic3 == "NULL" {
		l.topic3 = nil
	} else {
		t3, err := thor.ParseHash(dbLog.topic3)
		if err != nil {
			return nil, err
		}
		l.topic3 = &t3
	}
	if dbLog.topic4 == "NULL" {
		l.topic4 = nil
	} else {
		t4, err := thor.ParseHash(dbLog.topic4)
		if err != nil {
			return nil, err
		}
		l.topic4 = &t4
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
			topic4:      %v)`, log.blockID.String(),
		log.blockNumber,
		log.txID.String(),
		log.txOrigin.String(),
		log.address.String(),
		[]byte(log.data),
		formatHash(log.topic0),
		formatHash(log.topic1),
		formatHash(log.topic2),
		formatHash(log.topic3),
		formatHash(log.topic4))
}
