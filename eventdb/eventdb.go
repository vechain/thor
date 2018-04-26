package eventdb

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

//Filter filter
type Filter struct {
	Address  *thor.Address      `json:"address"` // always a contract address
	TopicSet [][5]*thor.Bytes32 `json:"topicSet"`
	Order    OrderType          `json:"order"` //default asc
	Range    *Range
	Options  *Options
}

//EventDB manages all events
type EventDB struct {
	path          string
	db            *sql.DB
	sqliteVersion string
}

//New open a event db
func New(path string) (*EventDB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(eventTableSchema); err != nil {
		return nil, err
	}
	s, _, _ := sqlite3.Version()
	return &EventDB{
		path:          path,
		db:            db,
		sqliteVersion: s,
	}, nil
}

//NewMem create a memory sqlite db
func NewMem() (*EventDB, error) {
	return New(":memory:")
}

//Insert insert events into db, and abandon events which associated with given block ids.
func (db *EventDB) Insert(events []*Event, abandonedBlockIDs []thor.Bytes32) error {
	if len(events) == 0 && len(abandonedBlockIDs) == 0 {
		return nil
	}
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	for _, event := range events {
		if _, err = tx.Exec("INSERT OR REPLACE INTO event(blockID ,eventIndex, blockNumber ,blockTime ,txID ,txOrigin ,address ,topic0 ,topic1 ,topic2 ,topic3 ,topic4, data) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?); ",
			event.BlockID.Bytes(),
			event.Index,
			event.BlockNumber,
			event.BlockTime,
			event.TxID.Bytes(),
			event.TxOrigin.Bytes(),
			event.Address.Bytes(),
			topicValue(event.Topics[0]),
			topicValue(event.Topics[1]),
			topicValue(event.Topics[2]),
			topicValue(event.Topics[3]),
			topicValue(event.Topics[4]),
			event.Data); err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, id := range abandonedBlockIDs {
		if _, err = tx.Exec("DELETE FROM event WHERE blockID = ?;", id.Bytes()); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

//Filter return events with options
func (db *EventDB) Filter(filter *Filter) ([]*Event, error) {
	if filter == nil {
		return db.query("SELECT * FROM event")
	}
	var args []interface{}
	stmt := "SELECT * FROM event WHERE 1"
	condition := "blockNumber"
	if filter.Range != nil {
		if filter.Range.Unit == Time {
			condition = "blockTime"
		}
		args = append(args, filter.Range.From)
		stmt += " AND " + condition + " >= ? "
		if filter.Range.To >= filter.Range.From {
			args = append(args, filter.Range.To)
			stmt += " AND " + condition + " <= ? "
		}
	}
	if filter.Address != nil {
		args = append(args, filter.Address.Bytes())
		stmt += " AND address = ? "
	}
	length := len(filter.TopicSet)
	if length > 0 {
		for i, topics := range filter.TopicSet {
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

	if filter.Order == DESC {
		stmt += " ORDER BY blockNumber,eventIndex DESC "
	} else {
		stmt += " ORDER BY blockNumber,eventIndex ASC "
	}

	if filter.Options != nil {
		stmt += " limit ?, ? "
		args = append(args, filter.Options.Offset, filter.Options.Limit)
	}
	return db.query(stmt, args...)
}

//query query events
func (db *EventDB) query(stmt string, args ...interface{}) ([]*Event, error) {
	rows, err := db.db.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var (
			blockID     []byte
			index       uint32
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
			&index,
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
		event := &Event{
			BlockID:     thor.BytesToBytes32(blockID),
			Index:       index,
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
				event.Topics[i] = &h
			}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

//Path return db's directory
func (db *EventDB) Path() string {
	return db.path
}

//Close close sqlite
func (db *EventDB) Close() {
	db.db.Close()
}

func topicValue(topic *thor.Bytes32) []byte {
	if topic == nil {
		return nil
	}
	return topic.Bytes()
}
