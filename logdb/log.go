package logdb

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//Log format tx.Log to store in db
type Log struct {
	blockID     thor.Hash
	blockNumber uint32
	txID        thor.Hash
	txOrigin    thor.Address //contract caller
	address     thor.Address // always a contract address
	data        []byte
	topic0      thor.Hash
	topic1      thor.Hash
	topic2      thor.Hash
	topic3      thor.Hash
	topic4      thor.Hash
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
		topic0:      thor.Hash{},
		topic1:      thor.Hash{},
		topic2:      thor.Hash{},
		topic3:      thor.Hash{},
		topic4:      thor.Hash{},
	}
	for i := 0; i < len(txLog.Topics); i++ {
		switch i {
		case 0:
			l.topic0 = txLog.Topics[0]
			break
		case 1:
			l.topic1 = txLog.Topics[1]
			break
		case 2:
			l.topic2 = txLog.Topics[2]
			break
		case 3:
			l.topic3 = txLog.Topics[3]
			break
		case 4:
			l.topic4 = txLog.Topics[4]
			break
		}
	}
	return l
}
