// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/big"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// the key to last written block id.
const (
	configBlockIDKey = "blockID"
	refIDQuery       = "(SELECT id FROM ref WHERE data=?)"
)

type LogDB struct {
	path          string
	db            *sql.DB
	driverVersion string
	stmtCache     *stmtCache
}

// New create or open log db at given path.
func New(path string) (logDB *LogDB, err error) {
	db, err := sql.Open("sqlite3", path+"?_journal=wal&cache=shared")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	if _, err := db.Exec(configTableSchema + refTableScheme + eventTableSchema + transferTableSchema); err != nil {
		return nil, err
	}

	driverVer, _, _ := sqlite3.Version()
	return &LogDB{
		path:          path,
		db:            db,
		driverVersion: driverVer,
		stmtCache:     newStmtCache(db),
	}, nil
}

// NewMem create a log db in ram.
func NewMem() (*LogDB, error) {
	return New("file::memory:")
}

// Close close the log db.
func (db *LogDB) Close() error {
	db.stmtCache.Clear()
	return db.db.Close()
}

func (db *LogDB) Path() string {
	return db.path
}

func (db *LogDB) FilterEvents(ctx context.Context, filter *EventFilter) ([]*Event, error) {

	const query = `SELECT e.seq, r0.data, e.blockTime, r1.data, r2.data, e.clauseIndex, r3.data, r4.data, r5.data, r6.data, r7.data, r8.data, e.data
FROM (%v) e
	LEFT JOIN ref r0 ON e.blockID = r0.id
	LEFT JOIN ref r1 ON e.txID = r1.id
	LEFT JOIN ref r2 ON e.txOrigin = r2.id
	LEFT JOIN ref r3 ON e.address = r3.id
	LEFT JOIN ref r4 ON e.topic0 = r4.id
	LEFT JOIN ref r5 ON e.topic1 = r5.id
	LEFT JOIN ref r6 ON e.topic2 = r6.id
	LEFT JOIN ref r7 ON e.topic3 = r7.id
	LEFT JOIN ref r8 ON e.topic4 = r8.id`

	if filter == nil {
		return db.queryEvents(ctx, fmt.Sprintf(query, "event"))
	}

	var (
		subQuery = "SELECT seq FROM event WHERE 1"
		args     []interface{}
	)

	if filter.Range != nil {
		subQuery += " AND seq >= ?"
		args = append(args, newSequence(filter.Range.From, 0))
		if filter.Range.To >= filter.Range.From {
			subQuery += " AND seq <= ?"
			args = append(args, newSequence(filter.Range.To, uint32(math.MaxInt32)))
		}
	}

	if len(filter.CriteriaSet) > 0 {
		subQuery += " AND ("

		for i, c := range filter.CriteriaSet {
			cond, cargs := c.toWhereCondition()
			if i > 0 {
				subQuery += " OR"
			}
			subQuery += " (" + cond + ")"
			args = append(args, cargs...)
		}
		subQuery += ")"
	}

	if filter.Order == DESC {
		subQuery += " ORDER BY seq DESC "
	} else {
		subQuery += " ORDER BY seq ASC "
	}

	if filter.Options != nil {
		subQuery += " LIMIT ?, ?"
		args = append(args, filter.Options.Offset, filter.Options.Limit)
	}

	subQuery = "SELECT e.* FROM (" + subQuery + ") s LEFT JOIN event e ON s.seq = e.seq"

	return db.queryEvents(ctx, fmt.Sprintf(query, subQuery), args...)
}

func (db *LogDB) FilterTransfers(ctx context.Context, filter *TransferFilter) ([]*Transfer, error) {

	const query = `SELECT t.seq, r0.data, t.blockTime, r1.data, r2.data, t.clauseIndex, r3.data, r4.data, t.amount
FROM (%v) t 
	LEFT JOIN ref r0 ON t.blockID = r0.id
	LEFT JOIN ref r1 ON t.txID = r1.id
	LEFT JOIN ref r2 ON t.txOrigin = r2.id
	LEFT JOIN ref r3 ON t.sender = r3.id
	LEFT JOIN ref r4 ON t.recipient = r4.id`

	if filter == nil {
		return db.queryTransfers(ctx, fmt.Sprintf(query, "transfer"))
	}

	var (
		subQuery = "SELECT seq FROM transfer WHERE 1"
		args     []interface{}
	)

	if filter.Range != nil {
		subQuery += " AND seq >= ?"
		args = append(args, newSequence(filter.Range.From, 0))
		if filter.Range.To >= filter.Range.From {
			subQuery += " AND seq <= ?"
			args = append(args, newSequence(filter.Range.To, uint32(math.MaxInt32)))
		}
	}

	if len(filter.CriteriaSet) > 0 {
		subQuery += " AND ("
		for i, c := range filter.CriteriaSet {
			cond, cargs := c.toWhereCondition()
			if i > 0 {
				subQuery += " OR"
			}
			subQuery += " (" + cond + ")"
			args = append(args, cargs...)
		}
		subQuery += ")"
	}

	if filter.Order == DESC {
		subQuery += " ORDER BY seq DESC"
	} else {
		subQuery += " ORDER BY seq ASC"
	}

	if filter.Options != nil {
		subQuery += " LIMIT ?, ?"
		args = append(args, filter.Options.Offset, filter.Options.Limit)
	}

	subQuery = "SELECT e.* FROM (" + subQuery + ") s LEFT JOIN transfer e ON s.seq = e.seq"
	return db.queryTransfers(ctx, fmt.Sprintf(query, subQuery), args...)
}

func (db *LogDB) queryEvents(ctx context.Context, query string, args ...interface{}) ([]*Event, error) {
	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var events []*Event
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var (
			seq         sequence
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
			&seq,
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
			BlockNumber: seq.BlockNumber(),
			Index:       seq.Index(),
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

func (db *LogDB) queryTransfers(ctx context.Context, query string, args ...interface{}) ([]*Transfer, error) {
	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var transfers []*Transfer
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var (
			seq         sequence
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
			&seq,
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
			BlockNumber: seq.BlockNumber(),
			Index:       seq.Index(),
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
	row := db.stmtCache.MustPrepare("SELECT value FROM config WHERE key=?").QueryRow(configBlockIDKey)
	var data []byte
	if err := row.Scan(&data); err != nil {
		if sql.ErrNoRows != err {
			return thor.Bytes32{}, err
		}

		// no config, query newest block ID from existing records
		row := db.stmtCache.MustPrepare(`SELECT MAX(data) FROM (
			SELECT data FROM ref WHERE id=(SELECT blockId FROM transfer ORDER BY seq DESC LIMIT 1)
			UNION
			SELECT data FROM ref WHERE id=(SELECT blockId FROM event ORDER BY seq DESC LIMIT 1))`).QueryRow()

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
	const query = `SELECT COUNT(*) FROM (
		SELECT * FROM (SELECT seq FROM transfer WHERE seq=? AND blockID=` + refIDQuery + ` LIMIT 1) 
		UNION
		SELECT * FROM (SELECT seq FROM event WHERE seq=? AND blockID=` + refIDQuery + ` LIMIT 1))`

	seq := newSequence(block.Number(id), 0)
	row := db.stmtCache.MustPrepare(query).QueryRow(seq, id.Bytes(), seq, id.Bytes())
	var count int
	if err := row.Scan(&count); err != nil {
		// no need to check ErrNoRows
		return false, err
	}
	return count > 0, nil
}

// Log write logs.
func (db *LogDB) Log(f func(*Writer) error) error {
	w := &Writer{db: db.db, stmtCache: db.stmtCache}
	if err := f(w); err != nil {
		if w.tx != nil {
			_ = w.tx.Rollback()
		}
		return err
	}
	return w.Flush()
}

func topicValue(topics []thor.Bytes32, i int) []byte {
	if i < len(topics) {
		return topics[i].Bytes()
	}
	return nil
}

// Writer is the transactional log writer.
type Writer struct {
	db          *sql.DB
	stmtCache   *stmtCache
	tx          *sql.Tx
	len         int
	lastBlockID thor.Bytes32
}

// Write writes all logs of the given block.
func (w *Writer) Write(b *block.Block, receipts tx.Receipts) error {

	var (
		num                       = b.Header().Number()
		id                        = b.Header().ID()
		ts                        = b.Header().Timestamp()
		txs                       = b.Transactions()
		eventCount, transferCount uint32
	)
	w.lastBlockID = id

	if num > 0 && w.len == 0 {
		seq := newSequence(num, 0)
		if err := w.exec("DELETE FROM event WHERE seq >= ?", seq); err != nil {
			return err
		}
		if err := w.exec("DELETE FROM transfer WHERE seq >= ?", seq); err != nil {
			return err
		}
	}

	if len(receipts) > 0 {
		if err := w.exec(
			"INSERT OR IGNORE INTO ref(data) VALUES(?)",
			id.Bytes()); err != nil {
			return err
		}

		for txIndex, receipt := range receipts {
			if len(receipt.Outputs) > 0 {
				var (
					txID     thor.Bytes32
					txOrigin thor.Address
				)
				if num != 0 {
					txID = txs[txIndex].ID()
					txOrigin, _ = txs[txIndex].Origin()
				}

				if err := w.exec(
					"INSERT OR IGNORE INTO ref(data) VALUES(?),(?)",
					txID.Bytes(), txOrigin.Bytes()); err != nil {
					return err
				}

				for clauseIndex, output := range receipt.Outputs {
					for _, ev := range output.Events {
						if err := w.exec(
							"INSERT OR IGNORE INTO ref (data) VALUES(?),(?),(?),(?),(?),(?)",
							ev.Address.Bytes(),
							topicValue(ev.Topics, 0),
							topicValue(ev.Topics, 1),
							topicValue(ev.Topics, 2),
							topicValue(ev.Topics, 3),
							topicValue(ev.Topics, 4)); err != nil {
							return err
						}

						if err := w.exec(
							fmt.Sprintf(
								"INSERT OR REPLACE INTO event VALUES(?,%v,?,%v,%v,?,%v,%v,%v,%v,%v,%v,?)",
								refIDQuery, refIDQuery, refIDQuery, refIDQuery, refIDQuery, refIDQuery, refIDQuery, refIDQuery, refIDQuery),
							newSequence(num, eventCount),
							id.Bytes(),
							ts,
							txID.Bytes(),
							txOrigin.Bytes(),
							clauseIndex,
							ev.Address.Bytes(),
							topicValue(ev.Topics, 0),
							topicValue(ev.Topics, 1),
							topicValue(ev.Topics, 2),
							topicValue(ev.Topics, 3),
							topicValue(ev.Topics, 4),
							ev.Data); err != nil {
							return err
						}

						eventCount++
					}

					for _, tr := range output.Transfers {
						if err := w.exec(
							"INSERT OR IGNORE INTO ref (data) VALUES(?),(?)",
							tr.Sender.Bytes(),
							tr.Recipient.Bytes()); err != nil {
							return err
						}
						if err := w.exec(
							fmt.Sprintf(
								"INSERT OR REPLACE INTO transfer VALUES(?,%v,?,%v,%v,?,%v,%v,?)",
								refIDQuery, refIDQuery, refIDQuery, refIDQuery, refIDQuery),
							newSequence(num, transferCount),
							id.Bytes(),
							ts,
							txID.Bytes(),
							txOrigin.Bytes(),
							clauseIndex,
							tr.Sender.Bytes(),
							tr.Recipient.Bytes(),
							tr.Amount.Bytes()); err != nil {
							return err
						}

						transferCount++
					}
				}
			}
		}
	}
	return nil
}

// Flush commits accumulated logs.
func (w *Writer) Flush() (err error) {
	if w.tx == nil {
		return nil
	}

	defer func() {
		if err != nil {
			_ = w.tx.Rollback()
		}
		w.tx = nil
		w.lastBlockID = thor.Bytes32{}
		w.len = 0
	}()

	if block.Number(w.lastBlockID) > 0 {
		if err := w.exec(
			"INSERT OR REPLACE INTO config(key, value) VALUES(?,?)",
			configBlockIDKey, w.lastBlockID.Bytes()); err != nil {
			return err
		}
	}
	return w.tx.Commit()
}

// Len returns count of pending logs.
func (w *Writer) Len() int {
	return w.len
}

func (w *Writer) exec(query string, args ...interface{}) error {
	if w.tx == nil {
		tx, err := w.db.Begin()
		if err != nil {
			return err
		}
		w.tx = tx
	}

	if _, err := w.tx.Stmt(w.stmtCache.MustPrepare(query)).Exec(args...); err != nil {
		return err
	}
	w.len++
	return nil
}
