// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
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

	metricsHandleEventsFilter(filter)

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
			cond, cargs := c.toWhereCondition()
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
		if filter.Order == DESC {
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
		if filter.Order == DESC {
			eventQuery += " ORDER BY seq DESC "
		} else {
			eventQuery += " ORDER BY seq ASC "
		}
	}
	return db.queryEvents(ctx, eventQuery, args...)
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

	metricsHandleCommonFilter(filter.Options, filter.Order, len(filter.CriteriaSet), "transfer")

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
			cond, cargs := c.toWhereCondition()
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
		if filter.Order == DESC {
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
		if filter.Order == DESC {
			transferQuery += " ORDER BY seq DESC "
		} else {
			transferQuery += " ORDER BY seq ASC "
		}
	}
	return db.queryTransfers(ctx, transferQuery, args...)
}

func (db *LogDB) queryEvents(ctx context.Context, query string, args ...any) ([]*Event, error) {
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

func (db *LogDB) queryTransfers(ctx context.Context, query string, args ...any) ([]*Transfer, error) {
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

func (db *LogDB) NewWriter() *Writer {
	return &Writer{
		conn:              db.wconn,
		stmtCache:         db.stmtCache,
		db:                db,
		connectionPool:    make(map[string]*sql.DB),
		partitions:        make(map[string]*sql.Conn),
		maxOpenPartitions: 3,                          // Add this line
		partitionLRU:      make(map[string]time.Time), // Add this line
	}
}

// NewWriterSyncOff creates a log writer which applied 'pragma synchronous = off'.
func (db *LogDB) NewWriterSyncOff() *Writer {
	return &Writer{
		conn:              db.wconnSyncOff,
		stmtCache:         db.stmtCache,
		db:                db,
		connectionPool:    make(map[string]*sql.DB),
		partitions:        make(map[string]*sql.Conn), // Add this line
		maxOpenPartitions: 3,                          // Add this line
		partitionLRU:      make(map[string]time.Time),
	}
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

// Writer is the transactional log writer.
type Writer struct {
	conn      *sql.Conn
	stmtCache *stmtCache

	tx               *sql.Tx
	uncommittedCount int

	// Partitioning support
	db                *LogDB
	partitionMux      sync.RWMutex
	currentPartition  int
	partitions        map[string]*sql.Conn
	connectionPool    map[string]*sql.DB
	poolMutex         sync.RWMutex
	maxOpenPartitions int
	partitionLRU      map[string]time.Time
}

// Truncate truncates the database by deleting logs after blockNum (included).
func (w *Writer) Truncate(blockNum uint32) error {
	seq, err := newSequence(blockNum, 0, 0)
	if err != nil {
		return err
	}

	if err := w.execWithTx("DELETE FROM event WHERE seq >= ?", seq); err != nil {
		return err
	}
	if err := w.execWithTx("DELETE FROM transfer WHERE seq >= ?", seq); err != nil {
		return err
	}
	return nil
}

func (w *Writer) Write(b *block.Block, receipts tx.Receipts) error {
	blockNum := b.Header().Number()
	partitionNum := int(blockNum / 500000)

	// Only switch partition if we're moving to a new one
	if w.currentPartition != partitionNum {
		if err := w.commitCurrentPartition(); err != nil {
			return err
		}
		if err := w.switchToPartition(partitionNum); err != nil {
			return err
		}
	}

	// Batch this block's data into current transaction
	return w.writeToDatabase(w.conn, b, receipts)
}

func (w *Writer) commitCurrentPartition() error {
	if w.tx != nil {
		if err := w.tx.Commit(); err != nil {
			return err
		}
		w.tx = nil
		w.uncommittedCount = 0
	}
	return nil
}

type PartitionManager struct {
	partitions     map[string]*sql.DB
	connections    map[string]*sql.Conn
	maxConnections int
}

//func (w *Writer) getConnection(partitionName string) *sql.Conn {
//	w.poolMutex.RLock()
//	if conn, exists := w.connectionPool[partitionName]; exists {
//		w.poolMutex.RUnlock()
//		return conn
//	}
//	w.poolMutex.RUnlock()
//
//	// Create and cache connection
//	w.poolMutex.Lock()
//	defer w.poolMutex.Unlock()
//
//	db := w.getPartitionDatabase(partitionName)
//	conn, err := db.Conn(context.Background())
//	if err != nil {
//		panic(fmt.Sprintf("Failed to get connection for partition %s: %v", partitionName, err))
//	}
//
//	w.connectionPool[partitionName] = conn
//	return conn
//}

func (w *Writer) switchToPartition(partitionNum int) error {
	partitionName := fmt.Sprintf("events_partition_%d", partitionNum)

	// Check if partition is already open
	if _, exists := w.partitions[partitionName]; exists {
		w.currentPartition = partitionNum
		w.conn = w.getConnection(partitionName) // This now returns *sql.Conn
		return nil
	}

	// Close oldest partition if we've hit the limit
	if len(w.partitions) >= w.maxOpenPartitions {
		w.evictOldestPartition()
	}

	// Create new partition
	db := w.createPartition(partitionName)
	w.connectionPool[partitionName] = db
	w.currentPartition = partitionNum
	w.conn = w.getConnection(partitionName) // This now returns *sql.Conn

	return nil
}

// getPartitionConnection gets or creates a partition database connection
//func (w *Writer) getPartitionConnection(partitionName string) *sql.Conn {
//	w.partitionMux.RLock()
//	if conn, exists := w.partitions[partitionName]; exists {
//		w.partitionMux.RUnlock()
//		return conn
//	}
//	w.partitionMux.RUnlock()
//
//	w.partitionMux.Lock()
//	defer w.partitionMux.Unlock()
//
//	// Double-check after acquiring write lock
//	if conn, exists := w.partitions[partitionName]; exists {
//		return conn
//	}
//
//	// Create partition database
//	partitionPath := filepath.Join(filepath.Dir(w.db.Path()), partitionName+".db")
//
//	db, err := sql.Open("sqlite3", partitionPath+"?_journal=wal&cache=shared")
//	if err != nil {
//		panic(fmt.Sprintf("Failed to create partition %s: %v", partitionName, err))
//	}
//
//	// Initialize schema
//	schema := refTableScheme + eventTableSchema + transferTableSchema
//	if _, err := db.Exec(schema); err != nil {
//		db.Close()
//		panic(fmt.Sprintf("Failed to initialize partition schema %s: %v", partitionName, err))
//	}
//
//	// Create connection and store it
//	conn, err := db.Conn(context.Background())
//	if err != nil {
//		db.Close()
//		panic(fmt.Sprintf("Failed to get connection for partition %s: %v", partitionName, err))
//	}
//
//	w.partitions[partitionName] = conn
//	return conn
//}

func (w *Writer) getPartitionDatabase(partitionName string) *sql.DB {
	w.partitionMux.RLock()
	if db, exists := w.connectionPool[partitionName]; exists {
		println("DEBUG: Reusing existing partition %s", partitionName)
		w.partitionMux.RUnlock()
		return db
	}
	w.partitionMux.RUnlock()

	w.partitionMux.Lock()
	defer w.partitionMux.Unlock()

	// Double-check after acquiring write lock
	if db, exists := w.connectionPool[partitionName]; exists {
		println("DEBUG: Reusing existing partition %s (double-check)", partitionName)
		return db
	}

	// Create partition database
	partitionPath := filepath.Join(filepath.Dir(w.db.Path()), partitionName+".db")
	println("DEBUG: Creating new partition %s at %s", partitionName, partitionPath)

	db, err := sql.Open("sqlite3", partitionPath+"?_journal=wal&cache=shared")
	if err != nil {
		println("ERROR: Failed to create partition %s: %v", partitionName, err)
		panic(fmt.Sprintf("Failed to create partition %s: %v", partitionName, err))
	}

	// Initialize schema
	schema := refTableScheme + eventTableSchema + transferTableSchema
	if _, err := db.Exec(schema); err != nil {
		println("ERROR: Failed to initialize partition schema %s: %v", partitionName, err)
		db.Close()
		panic(fmt.Sprintf("Failed to initialize partition schema %s: %v", partitionName, err))
	}

	w.connectionPool[partitionName] = db
	println("DEBUG: Created partition %s, total partitions: %d", partitionName, len(w.connectionPool))
	return db
}

func (w *Writer) writeToDatabase(conn *sql.Conn, b *block.Block, receipts tx.Receipts) error {
	// Start transaction if not already started
	if w.tx == nil {
		var err error
		if w.tx, err = conn.BeginTx(context.Background(), nil); err != nil {
			return err
		}
	}

	var (
		blockID        = b.Header().ID()
		blockNum       = b.Header().Number()
		blockTimestamp = b.Header().Timestamp()
		txs            = b.Transactions()
		isReceiptEmpty = func(r *tx.Receipt) bool {
			for _, o := range r.Outputs {
				if len(o.Events) > 0 || len(o.Transfers) > 0 {
					return false
				}
			}
			return true
		}
		blockIDInserted bool
	)

	eventCount, transferCount := uint32(0), uint32(0)
	for i, r := range receipts {
		if isReceiptEmpty(r) {
			continue
		}

		if !blockIDInserted {
			// block id is not yet inserted
			if err := w.execWithTx(
				"INSERT OR IGNORE INTO ref(data) VALUES(?)",
				blockID[:]); err != nil {
				return err
			}
			blockIDInserted = true
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

		txIndex := i
		if err := w.execWithTx(
			"INSERT OR IGNORE INTO ref(data) VALUES(?),(?)",
			txID[:], txOrigin[:]); err != nil {
			return err
		}

		for clauseIndex, output := range r.Outputs {
			for _, ev := range output.Events {
				if err := w.execWithTx(
					"INSERT OR IGNORE INTO ref (data) VALUES(?),(?),(?),(?),(?),(?)",
					ev.Address[:],
					topicValue(ev.Topics, 0),
					topicValue(ev.Topics, 1),
					topicValue(ev.Topics, 2),
					topicValue(ev.Topics, 3),
					topicValue(ev.Topics, 4),
				); err != nil {
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

				seq, err := newSequence(blockNum, uint32(txIndex), eventCount)
				if err != nil {
					return err
				}

				if err := w.execWithTx(
					query,
					seq,
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
				if err := w.execWithTx(
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

				seq, err := newSequence(blockNum, uint32(txIndex), transferCount)
				if err != nil {
					return err
				}

				if err := w.execWithTx(
					query,
					seq,
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

//func (w *Writer) exec(query string, args ...any) (err error) {
//	if w.tx == nil {
//		if w.tx, err = w.conn.BeginTx(context.Background(), nil); err != nil {
//			return
//		}
//	}
//	if _, err = w.tx.Stmt(w.stmtCache.MustPrepare(query)).Exec(args...); err != nil {
//		return
//	}
//	w.uncommittedCount++
//	return nil
//}

func (w *Writer) execWithTx(query string, args ...any) error {
	if w.tx == nil {
		trx, err := w.conn.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		w.tx = trx
	}
	if _, err := w.tx.Exec(query, args...); err != nil {
		return err
	}
	w.uncommittedCount++
	return nil
}

func (w *Writer) Close() error {
	w.partitionMux.Lock()
	defer w.partitionMux.Unlock()

	println("DEBUG: Closing writer, total partitions to close: %d", len(w.connectionPool))
	for name, db := range w.connectionPool {
		println("DEBUG: Closing partition %s", name)
		db.Close()
		delete(w.connectionPool, name)
	}
	w.connectionPool = make(map[string]*sql.DB)
	println("DEBUG: Writer closed, all partitions cleaned up")
	return nil
}

func (w *Writer) getConnection(partitionName string) *sql.Conn {
	w.poolMutex.RLock()
	if conn, exists := w.partitions[partitionName]; exists {
		w.poolMutex.RUnlock()
		return conn
	}
	w.poolMutex.RUnlock()

	// Create and cache connection
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	// Double-check after acquiring write lock
	if conn, exists := w.partitions[partitionName]; exists {
		return conn
	}

	db := w.getPartitionDatabase(partitionName)
	conn, err := db.Conn(context.Background())
	if err != nil {
		panic(fmt.Sprintf("Failed to get connection for partition %s: %v", partitionName, err))
	}

	w.partitions[partitionName] = conn
	return conn
}

func (w *Writer) createPartition(partitionName string) *sql.DB {
	partitionPath := filepath.Join(filepath.Dir(w.db.Path()), partitionName+".db")

	// Optimize settings for write-heavy workloads
	db, err := sql.Open("sqlite3", partitionPath+"?"+strings.Join([]string{
		"_journal=wal",
		"cache=shared",
		"synchronous=normal",
		"temp_store=memory",
		"mmap_size=268435456",
	}, "&"))

	if err != nil {
		panic(fmt.Sprintf("Failed to create partition %s: %v", partitionName, err))
	}

	// Set additional pragmas
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=normal")
	db.Exec("PRAGMA cache_size=10000")
	db.Exec("PRAGMA temp_store=memory")

	// Initialize schema - THIS IS THE KEY FIX
	schema := refTableScheme + eventTableSchema + transferTableSchema
	if _, err := db.Exec(schema); err != nil {
		println("ERROR: Failed to initialize partition schema %s: %v", partitionName, err)
		db.Close()
		panic(fmt.Sprintf("Failed to initialize partition schema %s: %v", partitionName, err))
	}

	println("DEBUG: Successfully created partition %s with schema", partitionName)
	return db
}

func (w *Writer) evictOldestPartition() {
	if len(w.connectionPool) < w.maxOpenPartitions {
		return
	}

	// Simple eviction: close the first partition that's not current
	for name, db := range w.connectionPool {
		if name != fmt.Sprintf("events_partition_%d", w.currentPartition) {
			println("DEBUG: Evicting partition %s", name)
			db.Close()
			delete(w.connectionPool, name)
			break
		}
	}
}
