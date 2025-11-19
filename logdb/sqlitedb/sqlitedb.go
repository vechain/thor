// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package sqlitedb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"

	"github.com/mattn/go-sqlite3"
)

const (
	refIDQuery = "(SELECT id FROM ref WHERE data=?)"
)

// SQLiteDB implements the LogDB interface using SQLite.
type SQLiteDB struct {
	path          string
	driverVersion string
	db            *sql.DB
	wconn         *sql.Conn
	wconnSyncOff  *sql.Conn
	stmtCache     *stmtCache
}

// New creates or opens a log database at the given path.
func New(path string) (logdb.LogDB, error) {
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
	return &SQLiteDB{
		path:          path,
		driverVersion: driverVer,
		db:            db,
		wconn:         wconn1,
		wconnSyncOff:  wconn2,
		stmtCache:     newStmtCache(db),
	}, nil
}

// NewMem creates a log database in memory.
// Unlike the file-based database, this implementation:
// 1. Uses synchronous=off for faster writes in memory
// 2. Uses a single connection with minimal transaction overhead
// 3. Optimizes for in-memory performance
func NewMem() (logdb.LogDB, error) {
	// Generate 6 random bytes for unique database name
	randBytes := make([]byte, 6)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	dbName := fmt.Sprintf("file:memdb_%s?mode=memory&cache=shared&_txlock=immediate&synchronous=off&journal_mode=memory", hex.EncodeToString(randBytes))
	// Open SQLite in-memory database with optimized settings:
	// - mode=memory: Create a new in-memory database
	// - cache=shared: Enable shared cache mode
	// - _txlock=immediate: Acquire locks immediately for better concurrency
	// - synchronous=off: Fastest mode for in-memory database
	// - journal_mode=memory: Use in-memory journal for better performance
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(refTableScheme + eventTableSchema + transferTableSchema); err != nil {
		db.Close()
		return nil, err
	}

	// Create a single connection with optimized settings
	wconn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, err
	}

	driverVer, _, _ := sqlite3.Version()
	return &SQLiteDB{
		path:          dbName,
		driverVersion: driverVer,
		db:            db,
		wconn:         wconn,
		wconnSyncOff:  wconn, // Use same connection for both writers
		stmtCache:     newStmtCache(db),
	}, nil
}

// Close closes the log database.
func (db *SQLiteDB) Close() (err error) {
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

func (db *SQLiteDB) Path() string {
	return db.path
}

func (db *SQLiteDB) FilterEvents(ctx context.Context, filter *logdb.EventFilter) ([]*logdb.Event, error) {
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

	logdb.MetricsHandleEventsFilter(filter)

	var (
		subQuery = "SELECT seq FROM event WHERE 1"
		args     []any
	)

	if filter.Range != nil {
		subQuery += " AND seq >= ?"
		from, err := newSequence(filter.Range.From, 0, 0)
		if err != nil {
			return nil, err
		}
		args = append(args, from)
		if filter.Range.To >= filter.Range.From {
			subQuery += " AND seq <= ?"
			to, err := newSequence(filter.Range.To, txIndexMask, logIndexMask)
			if err != nil {
				return nil, err
			}
			args = append(args, to)
		}
	}

	if len(filter.CriteriaSet) > 0 {
		subQuery += " AND ("

		for i, c := range filter.CriteriaSet {
			cond, cargs := c.ToWhereCondition()
			if i > 0 {
				subQuery += " OR"
			}
			subQuery += " (" + cond + ")"
			args = append(args, cargs...)
		}
		subQuery += ")"
	}

	// if there is limit option, set order inside subquery
	if filter.Options != nil {
		if filter.Order == logdb.DESC {
			subQuery += " ORDER BY seq DESC "
		} else {
			subQuery += " ORDER BY seq ASC "
		}
		subQuery += " LIMIT ?, ?"
		args = append(args, filter.Options.Offset, filter.Options.Limit)
	}

	subQuery = "SELECT e.* FROM (" + subQuery + ") s LEFT JOIN event e ON s.seq = e.seq"

	eventQuery := fmt.Sprintf(query, subQuery)
	// if there is no limit option, set order outside
	if filter.Options == nil {
		if filter.Order == logdb.DESC {
			eventQuery += " ORDER BY seq DESC "
		} else {
			eventQuery += " ORDER BY seq ASC "
		}
	}
	return db.queryEvents(ctx, eventQuery, args...)
}

func (db *SQLiteDB) FilterTransfers(ctx context.Context, filter *logdb.TransferFilter) ([]*logdb.Transfer, error) {
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

	logdb.MetricsHandleCommonFilter(filter.Options, filter.Order, len(filter.CriteriaSet), "transfer")

	var (
		subQuery = "SELECT seq FROM transfer WHERE 1"
		args     []any
	)

	if filter.Range != nil {
		subQuery += " AND seq >= ?"
		from, err := newSequence(filter.Range.From, 0, 0)
		if err != nil {
			return nil, err
		}
		args = append(args, from)
		if filter.Range.To >= filter.Range.From {
			subQuery += " AND seq <= ?"
			to, err := newSequence(filter.Range.To, txIndexMask, logIndexMask)
			if err != nil {
				return nil, err
			}
			args = append(args, to)
		}
	}

	if len(filter.CriteriaSet) > 0 {
		subQuery += " AND ("
		for i, c := range filter.CriteriaSet {
			cond, cargs := c.ToWhereCondition()
			if i > 0 {
				subQuery += " OR"
			}
			subQuery += " (" + cond + ")"
			args = append(args, cargs...)
		}
		subQuery += ")"
	}

	// if there is limit option, set order inside subquery
	if filter.Options != nil {
		if filter.Order == logdb.DESC {
			subQuery += " ORDER BY seq DESC"
		} else {
			subQuery += " ORDER BY seq ASC"
		}
		subQuery += " LIMIT ?, ?"
		args = append(args, filter.Options.Offset, filter.Options.Limit)
	}

	subQuery = "SELECT e.* FROM (" + subQuery + ") s LEFT JOIN transfer e ON s.seq = e.seq"
	transferQuery := fmt.Sprintf(query, subQuery)
	// if there is no limit option, set order outside
	if filter.Options == nil {
		if filter.Order == logdb.DESC {
			transferQuery += " ORDER BY seq DESC "
		} else {
			transferQuery += " ORDER BY seq ASC "
		}
	}
	return db.queryTransfers(ctx, transferQuery, args...)
}

func (db *SQLiteDB) queryEvents(ctx context.Context, query string, args ...any) ([]*logdb.Event, error) {
	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var events []*logdb.Event
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
		event := &logdb.Event{
			BlockNumber: seq.BlockNumber(),
			LogIndex:    seq.LogIndex(),
			BlockID:     thor.BytesToBytes32(blockID),
			BlockTime:   blockTime,
			TxID:        thor.BytesToBytes32(txID),
			TxIndex:     seq.TxIndex(),
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

func (db *SQLiteDB) queryTransfers(ctx context.Context, query string, args ...any) ([]*logdb.Transfer, error) {
	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var transfers []*logdb.Transfer
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
		trans := &logdb.Transfer{
			BlockNumber: seq.BlockNumber(),
			LogIndex:    seq.LogIndex(),
			BlockID:     thor.BytesToBytes32(blockID),
			BlockTime:   blockTime,
			TxID:        thor.BytesToBytes32(txID),
			TxIndex:     seq.TxIndex(),
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

// NewestBlockID queries the newest written block ID.
func (db *SQLiteDB) NewestBlockID() (thor.Bytes32, error) {
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

// HasBlockID queries whether given block ID related logs were written.
func (db *SQLiteDB) HasBlockID(id thor.Bytes32) (bool, error) {
	const query = `SELECT COUNT(*) FROM (
		SELECT * FROM (SELECT seq FROM transfer WHERE seq=? AND blockID=` + refIDQuery + ` LIMIT 1) 
		UNION
		SELECT * FROM (SELECT seq FROM event WHERE seq=? AND blockID=` + refIDQuery + ` LIMIT 1))`

	seq, err := newSequence(block.Number(id), 0, 0)
	if err != nil {
		return false, err
	}
	row := db.stmtCache.MustPrepare(query).QueryRow(seq, id[:], seq, id[:])
	var count int
	if err := row.Scan(&count); err != nil {
		// no need to check ErrNoRows
		return false, err
	}
	return count > 0, nil
}

// NewWriter creates a log writer.
func (db *SQLiteDB) NewWriter() logdb.Writer {
	return &Writer{conn: db.wconn, stmtCache: db.stmtCache}
}

// NewWriterSyncOff creates a log writer which applied 'pragma synchronous = off'.
func (db *SQLiteDB) NewWriterSyncOff() logdb.Writer {
	return &Writer{conn: db.wconnSyncOff, stmtCache: db.stmtCache}
}

func topicValue(topics []thor.Bytes32, i int) []byte {
	if i < len(topics) {
		return removeLeadingZeros(topics[i][:])
	}
	return nil
}

func removeLeadingZeros(bytes []byte) []byte {
	i := 0
	// increase i until it reaches the first non-zero byte
	for ; i < len(bytes) && bytes[i] == 0; i++ {
	}
	// ensure at least 1 byte exists
	if i == len(bytes) {
		return []byte{0}
	}
	return bytes[i:]
}
