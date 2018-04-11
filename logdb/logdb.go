package logdb

import (
	"database/sql"
	"fmt"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/vechain/thor/thor"
)

type RangeType string

const (
	Block RangeType = "Block"
	Time            = "Time"
)

type OrderType string

const (
	ASC  OrderType = "ASC"
	DESC           = "DESC"
)

type Range struct {
	Unit RangeType `json:"unit"`
	From uint64    `json:"from"`
	To   uint64    `json:"to"`
}

type Options struct {
	Offset uint64 `json:"offset"`
	Limit  uint64 `json:"limit"`
}

//LogFilter filter
type LogFilter struct {
	Address  *thor.Address      `json:"address"` // always a contract address
	TopicSet [][5]*thor.Bytes32 `json:"topicSet"`
	Order    OrderType          `json:"order"` //default asc
	Range    *Range
	Options  *Options
}

//LogDB manages all logs
type LogDB struct {
	path          string
	db            *sql.DB
	sqliteVersion string
}

//New open a logdb
func New(path string) (*LogDB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(logTableSchema); err != nil {
		return nil, err
	}
	s, _, _ := sqlite3.Version()
	return &LogDB{
		path:          path,
		db:            db,
		sqliteVersion: s,
	}, nil
}

//NewMem create a memory sqlite db
func NewMem() (*LogDB, error) {
	return New(":memory:")
}

//Insert insert logs into db, and abandon logs which associated with given block ids.
func (db *LogDB) Insert(logs []*Log, abandonedBlockIDs []thor.Bytes32) error {
	if len(logs) == 0 && len(abandonedBlockIDs) == 0 {
		return nil
	}
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	for _, log := range logs {
		if _, err = tx.Exec("INSERT OR REPLACE INTO log(blockID ,logIndex, blockNumber ,blockTime ,txID ,txOrigin ,address ,topic0 ,topic1 ,topic2 ,topic3 ,topic4, data) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?); ",
			log.BlockID.Bytes(),
			log.LogIndex,
			log.BlockNumber,
			log.BlockTime,
			log.TxID.Bytes(),
			log.TxOrigin.Bytes(),
			log.Address.Bytes(),
			topicValue(log.Topics[0]),
			topicValue(log.Topics[1]),
			topicValue(log.Topics[2]),
			topicValue(log.Topics[3]),
			topicValue(log.Topics[4]),
			log.Data); err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, id := range abandonedBlockIDs {
		if _, err = tx.Exec("DELETE FROM log WHERE blockID = ?;", id.Bytes()); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

//Filter return logs with options
func (db *LogDB) Filter(logFilter *LogFilter) ([]*Log, error) {
	if logFilter == nil {
		return db.query("SELECT * FROM log")
	}
	var args []interface{}
	stmt := "SELECT * FROM log WHERE 1"
	condition := "blockNumber"
	if logFilter.Range != nil {
		if logFilter.Range.Unit == Time {
			condition = "blockTime"
		}
		args = append(args, logFilter.Range.From)
		stmt += " AND " + condition + " >= ? "
		if logFilter.Range.To >= logFilter.Range.From {
			args = append(args, logFilter.Range.To)
			stmt += " AND " + condition + " <= ? "
		}
	}
	if logFilter.Address != nil {
		args = append(args, logFilter.Address.Bytes())
		stmt += " AND address = ? "
	}
	length := len(logFilter.TopicSet)
	if length > 0 {
		for i, topics := range logFilter.TopicSet {
			if i == 0 {
				stmt += " AND (( 1 "
			} else {
				stmt += " OR ( 1 "
			}
			for j, topic := range topics {
				if topic != nil {
					args = append(args, topic.Bytes())
					stmt += fmt.Sprintf(" AND topic%v = ? ", j)
				}
			}
			if i == length-1 {
				stmt += " )) "
			} else {
				stmt += " ) "
			}
		}
	}

	if logFilter.Order == DESC {
		stmt += " ORDER BY " + condition + " DESC "
	} else {
		stmt += " ORDER BY " + condition + " ASC "
	}

	if logFilter.Options != nil {
		stmt += " limit ?, ? "
		args = append(args, logFilter.Options.Offset, logFilter.Options.Limit)
	}
	return db.query(stmt, args...)
}

//query query logs
func (db *LogDB) query(stmt string, args ...interface{}) ([]*Log, error) {
	rows, err := db.db.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*Log
	for rows.Next() {
		var (
			blockID     []byte
			logIndex    uint32
			blockNumber uint32
			blockTime   uint64
			txID        []byte
			txOrigin    []byte
			address     []byte
			topics      [5][]byte
			data        []byte
		)
		if err := rows.Scan(
			&blockID,
			&logIndex,
			&blockNumber,
			&blockTime,
			&txID,
			&txOrigin,
			&address,
			&topics[0],
			&topics[1],
			&topics[2],
			&topics[3],
			&topics[4],
			&data,
		); err != nil {
			return nil, err
		}
		log := &Log{
			BlockID:     thor.BytesToBytes32(blockID),
			LogIndex:    logIndex,
			BlockNumber: blockNumber,
			BlockTime:   blockTime,
			TxID:        thor.BytesToBytes32(txID),
			TxOrigin:    thor.BytesToAddress(txOrigin),
			Address:     thor.BytesToAddress(address),
			Data:        data,
		}
		for i, topic := range topics {
			if len(topic) > 0 {
				h := thor.BytesToBytes32(topic)
				log.Topics[i] = &h
			}
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

//Path return db's directory
func (db *LogDB) Path() string {
	return db.path
}

//Close close sqlite
func (db *LogDB) Close() {
	db.db.Close()
}

func topicValue(topic *thor.Bytes32) []byte {
	if topic == nil {
		return nil
	}
	return topic.Bytes()
}
