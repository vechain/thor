// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// the key to last written block id.
var configBlockIDKey = "blockID"

type LogDB struct {
	path          string
	db            *sql.DB
	driverVersion string
}

// New create or open log db at given path.
func New(path string) (logDB *LogDB, err error) {
	db, err := sql.Open("sqlite3", path+"?_journal=wal&cache=shared")
	if err != nil {
		return nil, err
	}
	defer func() {
		if logDB == nil {
			db.Close()
		}
	}()

	// to avoid 'database is locked' error
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(configTableSchema + eventTableSchema + transferTableSchema); err != nil {
		return nil, err
	}

	driverVer, _, _ := sqlite3.Version()
	return &LogDB{
		path,
		db,
		driverVer,
	}, nil
}

// NewMem create a log db in ram.
func NewMem() (*LogDB, error) {
	return New(":memory:")
}

// Close close the log db.
func (db *LogDB) Close() {
	db.db.Close()
}

func (db *LogDB) Path() string {
	return db.path
}

// NewTask create a new task to perform transactional operations of
// writing logs.
func (db *LogDB) NewTask() *Task {
	return &Task{db: db.db}
}

func (db *LogDB) FilterEvents(ctx context.Context, filter *EventFilter) ([]*Event, error) {
	if filter == nil {
		return db.queryEvents(ctx, "SELECT * FROM event")
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
	if length := len(filter.CriteriaSet); length > 0 {
		for i, criteria := range filter.CriteriaSet {
			if i == 0 {
				stmt += " AND (( 1"
			} else {
				stmt += " OR ( 1"
			}
			if criteria.Address != nil {
				args = append(args, criteria.Address.Bytes())
				stmt += " AND address = ? "
			}
			for j, topic := range criteria.Topics {
				if topic != nil {
					args = append(args, topic.Bytes())
					stmt += fmt.Sprintf(" AND topic%v = ?", j)
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
		stmt += " ORDER BY blockNumber DESC,eventIndex DESC "
	} else {
		stmt += " ORDER BY blockNumber ASC,eventIndex ASC "
	}

	if filter.Options != nil {
		stmt += " limit ?, ? "
		args = append(args, filter.Options.Offset, filter.Options.Limit)
	}
	return db.queryEvents(ctx, stmt, args...)
}

func (db *LogDB) FilterTransfers(ctx context.Context, filter *TransferFilter) ([]*Transfer, error) {
	if filter == nil {
		return db.queryTransfers(ctx, "SELECT * FROM transfer")
	}
	var args []interface{}
	stmt := "SELECT * FROM transfer WHERE 1"
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
	if filter.TxID != nil {
		args = append(args, filter.TxID.Bytes())
		stmt += " AND txID = ? "
	}
	if length := len(filter.CriteriaSet); length > 0 {
		for i, criteria := range filter.CriteriaSet {
			if i == 0 {
				stmt += " AND (( 1 "
			} else {
				stmt += " OR ( 1 "
			}
			if criteria.TxOrigin != nil {
				args = append(args, criteria.TxOrigin.Bytes())
				stmt += " AND txOrigin = ? "
			}
			if criteria.Sender != nil {
				args = append(args, criteria.Sender.Bytes())
				stmt += " AND sender = ? "
			}
			if criteria.Recipient != nil {
				args = append(args, criteria.Recipient.Bytes())
				stmt += " AND recipient = ? "
			}
			if i == length-1 {
				stmt += " )) "
			} else {
				stmt += " ) "
			}
		}
	}
	if filter.Order == DESC {
		stmt += " ORDER BY blockNumber DESC,transferIndex DESC "
	} else {
		stmt += " ORDER BY blockNumber ASC,transferIndex ASC "
	}
	if filter.Options != nil {
		stmt += " limit ?, ? "
		args = append(args, filter.Options.Offset, filter.Options.Limit)
	}
	return db.queryTransfers(ctx, stmt, args...)
}

func (db *LogDB) queryEvents(ctx context.Context, stmt string, args ...interface{}) ([]*Event, error) {
	rows, err := db.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var (
			blockNumber uint32
			index       uint32
			blockID     []byte
			blockTime   uint64
			txID        []byte
			txOrigin    []byte
			clauseIndex uint32
			address     []byte
			topics      [5][]byte
			data        []byte
		)
		if err := rows.Scan(
			&blockNumber,
			&index,
			&blockID,
			&blockTime,
			&txID,
			&txOrigin,
			&clauseIndex,
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
			BlockNumber: blockNumber,
			Index:       index,
			BlockID:     thor.BytesToBytes32(blockID),
			BlockTime:   blockTime,
			TxID:        thor.BytesToBytes32(txID),
			TxOrigin:    thor.BytesToAddress(txOrigin),
			ClauseIndex: clauseIndex,
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

func (db *LogDB) queryTransfers(ctx context.Context, stmt string, args ...interface{}) ([]*Transfer, error) {
	rows, err := db.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var transfers []*Transfer
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var (
			blockNumber uint32
			index       uint32
			blockID     []byte
			blockTime   uint64
			txID        []byte
			txOrigin    []byte
			clauseIndex uint32
			sender      []byte
			recipient   []byte
			amount      []byte
		)
		if err := rows.Scan(
			&blockNumber,
			&index,
			&blockID,
			&blockTime,
			&txID,
			&txOrigin,
			&clauseIndex,
			&sender,
			&recipient,
			&amount,
		); err != nil {
			return nil, err
		}
		trans := &Transfer{
			BlockNumber: blockNumber,
			Index:       index,
			BlockID:     thor.BytesToBytes32(blockID),
			BlockTime:   blockTime,
			TxID:        thor.BytesToBytes32(txID),
			TxOrigin:    thor.BytesToAddress(txOrigin),
			ClauseIndex: clauseIndex,
			Sender:      thor.BytesToAddress(sender),
			Recipient:   thor.BytesToAddress(recipient),
			Amount:      new(big.Int).SetBytes(amount),
		}
		transfers = append(transfers, trans)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return transfers, nil
}

// NewestBlockID query newest written block id.
func (db *LogDB) NewestBlockID() (thor.Bytes32, error) {
	// select from config if any
	row := db.db.QueryRow("SELECT value FROM config WHERE key=?", configBlockIDKey)
	var data []byte
	if err := row.Scan(&data); err != nil {
		if sql.ErrNoRows != err {
			return thor.Bytes32{}, err
		}

		// no config, query newest block ID from existing records
		row = db.db.QueryRow(
			`SELECT MAX(blockId) FROM (
SELECT * FROM (SELECT blockId FROM transfer ORDER BY blockNumber DESC LIMIT 1)
UNION
SELECT * FROM (SELECT blockId FROM event ORDER BY blockNumber DESC LIMIT 1))`)

		if err := row.Scan(&data); err != nil {
			if sql.ErrNoRows != err {
				return thor.Bytes32{}, err
			}
		}
	}
	return thor.BytesToBytes32(data), nil
}

// HasBlockID query whether given block id related logs were written.
func (db *LogDB) HasBlockID(id thor.Bytes32) (bool, error) {
	num := block.Number(id)
	row := db.db.QueryRow(`SELECT COUNT(*) FROM (
SELECT * FROM (SELECT blockNumber FROM transfer WHERE blockNumber=? AND blockID=? LIMIT 1) 
UNION
SELECT * FROM (SELECT blockNumber FROM event WHERE blockNumber=? AND blockID=? LIMIT 1))`,
		num, id.Bytes(), num, id.Bytes())
	var count int
	if err := row.Scan(&count); err != nil {
		// no need to check ErrNoRows
		return false, err
	}
	return count > 0, nil
}

func topicValue(topics []thor.Bytes32, i int) []byte {
	if i < len(topics) {
		return topics[i].Bytes()
	}
	return nil
}

// Task to transactionally perform logs writting.
type Task struct {
	db      *sql.DB
	queries []func(tx *sql.Tx) error
	ctx     *taskContext
}

type taskContext struct {
	blockNumber uint32
	blockID     thor.Bytes32
	blockTime   uint64

	// counters
	eventCount    uint32
	transferCount uint32
}

func (t *Task) add(op func(tx *sql.Tx) error) {
	t.queries = append(t.queries, op)
}

// ForBlock set context to given block.
func (t *Task) ForBlock(b *block.Header) *Task {
	if b.Number() > 0 {
		t.add(func(tx *sql.Tx) error {
			if _, err := tx.Exec("DELETE from event where blockNumber >= ?", b.Number()); err != nil {
				return err
			}
			if _, err := tx.Exec("DELETE from transfer where blockNumber >= ?", b.Number()); err != nil {
				return err
			}

			if _, err := tx.Exec("INSERT OR REPLACE INTO config(key, value) VALUES(?,?)",
				configBlockIDKey,
				b.ID().Bytes()); err != nil {
				return err
			}
			return nil
		})
	}
	t.ctx = &taskContext{
		blockNumber: b.Number(),
		blockID:     b.ID(),
		blockTime:   b.Timestamp(),
	}
	return t
}

// Write write all outputs of a tx.
func (t *Task) Write(txID thor.Bytes32, txOrigin thor.Address, outputs []*tx.Output) *Task {
	// necessary to assign since it's used in closure
	ctx := t.ctx
	t.add(func(tx *sql.Tx) error {
		for clauseIndex, output := range outputs {
			for _, ev := range output.Events {
				if _, err := tx.Exec("INSERT OR REPLACE INTO event(blockNumber, eventIndex, blockID, blockTime, txID, txOrigin, clauseIndex, address, topic0, topic1, topic2, topic3, topic4, data) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);",
					ctx.blockNumber,
					ctx.eventCount,
					ctx.blockID.Bytes(),
					ctx.blockTime,
					txID.Bytes(),
					txOrigin.Bytes(),
					clauseIndex,
					ev.Address.Bytes(),
					topicValue(ev.Topics, 0),
					topicValue(ev.Topics, 1),
					topicValue(ev.Topics, 2),
					topicValue(ev.Topics, 3),
					topicValue(ev.Topics, 4),
					ev.Data,
				); err != nil {
					return err
				}
				ctx.eventCount++
			}
			for _, tr := range output.Transfers {
				if _, err := tx.Exec("INSERT OR REPLACE INTO transfer(blockNumber, transferIndex, blockID, blockTime, txID, txOrigin, clauseIndex, sender, recipient, amount) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);",
					ctx.blockNumber,
					ctx.transferCount,
					ctx.blockID.Bytes(),
					ctx.blockTime,
					txID.Bytes(),
					txOrigin.Bytes(),
					clauseIndex,
					tr.Sender.Bytes(),
					tr.Recipient.Bytes(),
					tr.Amount.Bytes(),
				); err != nil {
					return err
				}
				ctx.transferCount++
			}
		}
		return nil
	})
	return t
}

// Commit commit task.
func (t *Task) Commit() error {
	tx, err := t.db.Begin()
	if err != nil {
		return err
	}
	for _, q := range t.queries {
		if err := q(tx); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
