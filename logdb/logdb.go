package logdb

import (
	"database/sql"
	"fmt"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/vechain/thor/thor"
	"sync"
)

//FilterOption option filter
type FilterOption struct {
	FromBlock uint32
	ToBlock   uint32
	Address   thor.Address // always a contract address
	Topics    [5]thor.Hash
}

//DB manages all logs
type DB struct {
	path          string
	db            *sql.DB
	sqliteVersion string
	m             sync.RWMutex
}

//OpenDB open a logdb
func OpenDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	s, _, _ := sqlite3.Version()
	if err != nil {
		return nil, err
	}
	return &DB{
		path:          path,
		db:            db,
		sqliteVersion: s,
	}, nil
}

//Insert insert logs into db
func (db *DB) Insert(logs []*Log) error {
	if len(logs) == 0 {
		return nil
	}
	db.m.Lock()
	defer db.m.Unlock()

	stmt := ""
	for _, log := range logs {
		stmt += "insert into log(blockID ,blockNumber ,txID ,txOrigin ,address ,data ,topic0 ,topic1 ,topic2 ,topic3 ,topic4) values " + fmt.Sprintf(" ('%v',%v,'%v','%v','%v','%s','%v','%v','%v','%v','%v'); ", log.blockID.String(),
			log.blockNumber,
			log.txID.String(),
			log.txOrigin.String(),
			log.address.String(),
			string(log.data),
			log.topic0.String(),
			log.topic1.String(),
			log.topic2.String(),
			log.topic3.String(),
			log.topic4.String())
	}
	return db.ExecInTransaction(stmt)
}

//Filter return logs with options
func (db *DB) Filter(options []*FilterOption) ([]*Log, error) {
	stmt := "select * from log where 1 "
	if len(options) != 0 {
		for _, op := range options {
			stmt += fmt.Sprintf(" or (blockNumber >= %v and blockNumber <= %v and address = %v and topic0 = %v and topic1 = %v and topic2 = %v and topic3 = %v and topic4 = %v ) ", op.FromBlock, op.ToBlock, op.Address, op.Topics[0], op.Topics[1], op.Topics[2], op.Topics[3], op.Topics[4])
		}
	}
	return db.Query(stmt)
}

//ExecInTransaction execute sql in a transaction
func (db *DB) ExecInTransaction(sqlStmt string, args ...interface{}) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(sqlStmt, args...); err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil
}

//Query query logs
func (db *DB) Query(stmt string) ([]*Log, error) {
	db.m.RLock()
	defer db.m.RUnlock()

	rows, err := db.db.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*Log
	for rows.Next() {
		var blockID string
		var blockNumber uint32
		var txID string
		var txOrigin string
		var address string
		var data string
		var topic0 string
		var topic1 string
		var topic2 string
		var topic3 string
		var topic4 string
		err = rows.Scan(&blockID, &blockNumber, &txID, &txOrigin, &address, &data, &topic0, &topic1, &topic2, &topic3, &topic4)
		if err != nil {
			return nil, err
		}
		log, err := CreateLog(blockID, blockNumber, txID, txOrigin, address, data, topic0, topic1, topic2, topic3, topic4)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return logs, nil
}

//Path return db's directory
func (db *DB) Path() string {
	return db.path
}
