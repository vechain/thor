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

const (
	refIDQuery = "(SELECT id FROM ref WHERE data=?)"
)

type LogDB struct {
	path          string
	driverVersion string
	db            *sql.DB
	wconn         *sql.Conn
	wconnSyncOff  *sql.Conn
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

	if _, err := db.Exec(refTableScheme + eventTableSchema + transferTableSchema); err != nil {
		return nil, err
	}

	wconn1, err := db.Conn(context.Background())
	if err != nil {
		return nil, err
	}

	wconn2, err := db.Conn(context.Background())
	if err != nil {
		return nil, err
	}

	if _, err := wconn2.ExecContext(context.Background(), "pragma synchronous=off"); err != nil {
		return nil, err
	}

	driverVer, _, _ := sqlite3.Version()
	return &LogDB{
		path:          path,
		driverVersion: driverVer,
		db:            db,
		wconn:         wconn1,
		wconnSyncOff:  wconn2,
		stmtCache:     newStmtCache(db),
	}, nil
}

// NewMem create a log db in ram.
func NewMem() (*LogDB, error) {
	return New("file::memory:")
}

// Close close the log db.
func (db *LogDB) Close() (err error) {
	err = db.wconn.Close()
	if err1 := db.wconnSyncOff.Close(); err == nil {
		err = err1
	}
	db.stmtCache.Clear()
	if err1 := db.db.Close(); err == nil {
		err = err1
	}
	return err
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
	var data []byte
	row := db.stmtCache.MustPrepare(`SELECT MAX(data) FROM (
			SELECT data FROM ref WHERE id=(SELECT blockId FROM transfer ORDER BY seq DESC LIMIT 1)
			UNION
			SELECT data FROM ref WHERE id=(SELECT blockId FROM event ORDER BY seq DESC LIMIT 1))`).QueryRow()

	if err := row.Scan(&data); err != nil {
		if sql.ErrNoRows != err {
			return thor.Bytes32{}, err
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
	row := db.stmtCache.MustPrepare(query).QueryRow(seq, id[:], seq, id[:])
	var count int
	if err := row.Scan(&count); err != nil {
		// no need to check ErrNoRows
		return false, err
	}
	return count > 0, nil
}

// NewWriter creates a log writer.
func (db *LogDB) NewWriter() *Writer {
	return &Writer{conn: db.wconn, stmtCache: db.stmtCache}
}

// NewWriterSyncOff creates a log writer which applied 'pragma synchronous = off'.
func (db *LogDB) NewWriterSyncOff() *Writer {
	return &Writer{conn: db.wconnSyncOff, stmtCache: db.stmtCache}
}

func topicValue(topics []thor.Bytes32, i int) []byte {
	if i < len(topics) {
		return topics[i][:]
	}
	return nil
}

// Writer is the transactional log writer.
type Writer struct {
	conn      *sql.Conn
	stmtCache *stmtCache

	tx               *sql.Tx
	uncommittedCount int
}

// Truncate truncates the database by deleting logs after blockNum (included).
func (w *Writer) Truncate(blockNum uint32) error {
	seq := newSequence(blockNum, 0)
	if err := w.exec("DELETE FROM event WHERE seq >= ?", seq); err != nil {
		return err
	}
	if err := w.exec("DELETE FROM transfer WHERE seq >= ?", seq); err != nil {
		return err
	}
	return nil
}

// Write writes all logs of the given block.
func (w *Writer) Write(b *block.Block, receipts tx.Receipts) error {
	var (
		blockID        = b.Header().ID()
		blockNum       = b.Header().Number()
		blockTimestamp = b.Header().Timestamp()
		txs            = b.Transactions()
		eventCount,
		transferCount uint32
		isReceiptEmpty = func(r *tx.Receipt) bool {
			for _, o := range r.Outputs {
				if len(o.Events) > 0 || len(o.Transfers) > 0 {
					return false
				}
			}
			return true
		}
	)

	for i, r := range receipts {
		if isReceiptEmpty(r) {
			continue
		}

		if eventCount == 0 && transferCount == 0 {
			// block id is not yet inserted
			if err := w.exec(
				"INSERT OR IGNORE INTO ref(data) VALUES(?)",
				blockID[:]); err != nil {
				return err
			}
		}

		var (
			txID     thor.Bytes32
			txOrigin thor.Address
		)
		if i < len(txs) { // block 0 has no tx, but has receipts
			tx := txs[i]
			txID = tx.ID()
			txOrigin, _ = tx.Origin()

		}
		if err := w.exec(
			"INSERT OR IGNORE INTO ref(data) VALUES(?),(?)",
			txID[:], txOrigin[:]); err != nil {
			return err
		}

		for clauseIndex, output := range r.Outputs {
			for _, ev := range output.Events {
				if err := w.exec(
					"INSERT OR IGNORE INTO ref (data) VALUES(?),(?),(?),(?),(?),(?)",
					ev.Address[:],
					topicValue(ev.Topics, 0),
					topicValue(ev.Topics, 1),
					topicValue(ev.Topics, 2),
					topicValue(ev.Topics, 3),
					topicValue(ev.Topics, 4)); err != nil {
					return err
				}

				const query = "INSERT OR IGNORE INTO event(seq, blockTime, clauseIndex, data, blockID, txID, txOrigin, address, topic0, topic1, topic2, topic3, topic4) " +
					"VALUES(?,?,?,?," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + ")"

				var eventData []byte
				if len(ev.Data) > 0 {
					eventData = ev.Data
				}

				if err := w.exec(
					query,
					newSequence(blockNum, eventCount),
					blockTimestamp,
					clauseIndex,
					eventData,
					blockID[:],
					txID[:],
					txOrigin[:],
					ev.Address[:],
					topicValue(ev.Topics, 0),
					topicValue(ev.Topics, 1),
					topicValue(ev.Topics, 2),
					topicValue(ev.Topics, 3),
					topicValue(ev.Topics, 4)); err != nil {
					return err
				}
				eventCount++
			}

			for _, tr := range output.Transfers {
				if err := w.exec(
					"INSERT OR IGNORE INTO ref (data) VALUES(?),(?)",
					tr.Sender[:],
					tr.Recipient[:]); err != nil {
					return err
				}
				const query = "INSERT OR IGNORE INTO transfer(seq, blockTime, clauseIndex, amount, blockID, txID, txOrigin, sender, recipient) " +
					"VALUES(?,?,?,?," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + ")"

				if err := w.exec(
					query,
					newSequence(blockNum, transferCount),
					blockTimestamp,
					clauseIndex,
					tr.Amount.Bytes(),
					blockID[:],
					txID[:],
					txOrigin[:],
					tr.Sender[:],
					tr.Recipient[:]); err != nil {
					return err
				}
				transferCount++
			}
		}
	}
	return nil
}

// Commit commits accumulated logs.
func (w *Writer) Commit() (err error) {
	if w.tx == nil {
		return nil
	}

	defer func() {
		if err == nil {
			w.tx = nil
			w.uncommittedCount = 0
		}
	}()
	return w.tx.Commit()
}

// Rollback rollback all uncommitted logs.
func (w *Writer) Rollback() (err error) {
	if w.tx == nil {
		return nil
	}
	defer func() {
		if err == nil {
			w.tx = nil
			w.uncommittedCount = 0
		}
	}()
	return w.tx.Rollback()
}

// UncommittedCount returns the count of uncommitted logs.
func (w *Writer) UncommittedCount() int {
	return w.uncommittedCount
}

func (w *Writer) exec(query string, args ...interface{}) (err error) {
	if w.tx == nil {
		if w.tx, err = w.conn.BeginTx(context.Background(), nil); err != nil {
			return
		}
	}
	if _, err = w.tx.Stmt(w.stmtCache.MustPrepare(query)).Exec(args...); err != nil {
		return
	}
	w.uncommittedCount++
	return nil
}
