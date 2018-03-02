package logdb

import (
	"database/sql"
	"fmt"
	sqlite3 "github.com/mattn/go-sqlite3"
	"sync"
)

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

//Store store logs
func (db *DB) Store(logs []*Log) error {
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
