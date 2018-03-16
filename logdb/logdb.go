package logdb

import (
	"database/sql"
	"fmt"
	"sync"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/vechain/thor/thor"
)

//FilterOption option filter
type FilterOption struct {
	FromBlock uint32
	ToBlock   uint32
	Address   *thor.Address // always a contract address
	TopicSet  [][5]*thor.Hash
}

//LogDB manages all logs
type LogDB struct {
	path          string
	db            *sql.DB
	sqliteVersion string
	m             sync.RWMutex
}

//New open a logdb
func New(path string) (*LogDB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	s, _, _ := sqlite3.Version()
	logDB := &LogDB{
		path:          path,
		db:            db,
		sqliteVersion: s,
	}
	err = logDB.execInTransaction(logTableSchema)
	if err != nil {
		return nil, err
	}
	return logDB, nil
}

//NewMem create a memory sqlite db
func NewMem() (*LogDB, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	s, _, _ := sqlite3.Version()
	ldb := &LogDB{
		db:            db,
		sqliteVersion: s,
	}
	if err = ldb.execInTransaction(logTableSchema); err != nil {
		return nil, err
	}
	return ldb, nil
}

//Insert insert logs into db
func (db *LogDB) Insert(logs []*Log) error {
	if len(logs) == 0 {
		return nil
	}
	db.m.Lock()
	defer db.m.Unlock()

	stmt := ""
	for _, log := range logs {
		stmt += "insert into log(blockID ,blockNumber ,logIndex ,txID ,txOrigin ,address ,data ,topic0 ,topic1 ,topic2 ,topic3 ,topic4) values " + fmt.Sprintf(" ('%v',%v, %v,'%v','%v','%v','%s','%v','%v','%v','%v','%v'); ",
			log.BlockID,
			log.BlockNumber,
			log.LogIndex,
			log.TxID,
			log.TxOrigin,
			log.Address,
			string(log.Data),
			formatHash(log.Topics[0]),
			formatHash(log.Topics[1]),
			formatHash(log.Topics[2]),
			formatHash(log.Topics[3]),
			formatHash(log.Topics[4]))
	}
	return db.execInTransaction(stmt)
}

//Filter return logs with options
func (db *LogDB) Filter(option *FilterOption) ([]*Log, error) {
	if option == nil {
		return db.Query("select * from log")
	}
	stmt := "select * from log where ( 1"
	stmt += fmt.Sprintf(" and blockNumber >= %v ", option.FromBlock)
	if option.ToBlock >= option.FromBlock {
		stmt += fmt.Sprintf(" and blockNumber <= %v ", option.ToBlock)
	}
	if option.Address != nil {
		stmt += fmt.Sprintf(" and address = '%v' ", option.Address)
	}
	stmt += " ) "
	length := len(option.TopicSet)
	if length > 0 {
		for i, topics := range option.TopicSet {
			if i == 0 {
				stmt += " and (( 1 "
			} else {
				stmt += " or ( 1 "
			}
			if topics[0] != nil {
				stmt += fmt.Sprintf(" and topic0 = '%v' ", topics[0])
			}
			if topics[1] != nil {
				stmt += fmt.Sprintf(" and topic1 = '%v' ", topics[1])
			}
			if topics[2] != nil {
				stmt += fmt.Sprintf(" and topic2 = '%v' ", topics[2])
			}
			if topics[3] != nil {
				stmt += fmt.Sprintf(" and topic3 = '%v' ", topics[3])
			}
			if topics[4] != nil {
				stmt += fmt.Sprintf(" and topic4 = '%v' ", topics[4])
			}
			if i == length-1 {
				stmt += " )) "
			} else {
				stmt += " ) "
			}
		}
	}
	return db.Query(stmt)
}

//execInTransaction execute sql in a transaction
func (db *LogDB) execInTransaction(sqlStmt string, args ...interface{}) error {
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
func (db *LogDB) Query(stmt string) ([]*Log, error) {
	db.m.RLock()
	defer db.m.RUnlock()
	rows, err := db.db.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*Log
	for rows.Next() {
		dbLog := &DBLog{}
		err = rows.Scan(
			&dbLog.blockID,
			&dbLog.blockNumber,
			&dbLog.logIndex,
			&dbLog.txID,
			&dbLog.txOrigin,
			&dbLog.address,
			&dbLog.data,
			&dbLog.topic0,
			&dbLog.topic1,
			&dbLog.topic2,
			&dbLog.topic3,
			&dbLog.topic4)
		if err != nil {
			return nil, err
		}

		log, err := dbLog.toLog()
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
func (db *LogDB) Path() string {
	return db.path
}
