package transferdb

import (
	"database/sql"
	"math/big"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/vechain/thor/thor"
)

//TransferDB manages transfers
type TransferDB struct {
	path          string
	db            *sql.DB
	sqliteVersion string
}

//New open a logdb
func New(path string) (*TransferDB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(transferTableSchema); err != nil {
		return nil, err
	}
	s, _, _ := sqlite3.Version()
	return &TransferDB{
		path:          path,
		db:            db,
		sqliteVersion: s,
	}, nil
}

//NewMem create a memory sqlite db
func NewMem() (*TransferDB, error) {
	return New(":memory:")
}

//Insert insert logs into db, and abandon logs which associated with given block ids.
func (db *TransferDB) Insert(transfers []*Transfer, abandonedBlockIDs []thor.Bytes32) error {
	if len(transfers) == 0 && len(transfers) == 0 {
		return nil
	}
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	for _, trans := range transfers {
		if _, err = tx.Exec("INSERT OR REPLACE INTO transfer(blockID ,transferIndex, blockNumber ,blockTime ,txID ,fromAddress ,toAddress ,value) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?); ",
			trans.BlockID.Bytes(),
			trans.TransferIndex,
			trans.BlockNumber,
			trans.BlockTime,
			trans.TxID.Bytes(),
			trans.From.Bytes(),
			trans.To.Bytes(),
			trans.Value.Bytes()); err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, id := range abandonedBlockIDs {
		if _, err = tx.Exec("DELETE FROM transfer WHERE blockID = ?;", id.Bytes()); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

//query query logs
func (db *TransferDB) query(stmt string, args ...interface{}) ([]*Transfer, error) {
	rows, err := db.db.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []*Transfer
	for rows.Next() {
		var (
			blockID       []byte
			transferIndex uint32
			blockNumber   uint32
			blockTime     uint64
			txID          []byte
			from          []byte
			to            []byte
			value         []byte
		)
		if err := rows.Scan(
			&blockID,
			&transferIndex,
			&blockNumber,
			&blockTime,
			&txID,
			&from,
			&to,
			&value,
		); err != nil {
			return nil, err
		}
		trans := &Transfer{
			BlockID:       thor.BytesToBytes32(blockID),
			TransferIndex: transferIndex,
			BlockNumber:   blockNumber,
			BlockTime:     blockTime,
			TxID:          thor.BytesToBytes32(txID),
			From:          thor.BytesToAddress(from),
			To:            thor.BytesToAddress(to),
			Value:         new(big.Int).SetBytes(value),
		}
		transfers = append(transfers, trans)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return transfers, nil
}

//Path return db's directory
func (db *TransferDB) Path() string {
	return db.path
}

//Close close sqlite
func (db *TransferDB) Close() {
	db.db.Close()
}
