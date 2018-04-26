package transferdb

import (
	"database/sql"
	"math/big"

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

type AddressSet struct {
	TxOrigin *thor.Address `json:"txOrigin"` //who send transaction
	From     *thor.Address `json:"from"`     //who transferred tokens
	To       *thor.Address `json:"to"`       //who recieved tokens
}

type TransferFilter struct {
	TxID        *thor.Bytes32 `json:"txID"`
	AddressSets []*AddressSet `json:"addressSets"`
	Range       *Range        `json:"range"`
	Options     *Options      `json:"options"`
	Order       OrderType     `json:"order"` //default asc
}

//TransferDB manages transfers
type TransferDB struct {
	path          string
	db            *sql.DB
	sqliteVersion string
}

//New open a transfer db
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

//Insert insert transfer logs into db, and abandon transfer logs which associated with given block ids.
func (db *TransferDB) Insert(transfers []*Transfer, abandonedBlockIDs []thor.Bytes32) error {
	if len(transfers) == 0 && len(transfers) == 0 {
		return nil
	}
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	for _, trans := range transfers {
		if _, err = tx.Exec("INSERT OR REPLACE INTO transfer(blockID ,transferIndex, blockNumber ,blockTime ,txID ,txOrigin ,fromAddress ,toAddress ,value) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?); ",
			trans.BlockID.Bytes(),
			trans.Index,
			trans.BlockNumber,
			trans.BlockTime,
			trans.TxID.Bytes(),
			trans.TxOrigin.Bytes(),
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

func (db *TransferDB) Filter(transferFilter *TransferFilter) ([]*Transfer, error) {
	if transferFilter == nil {
		return db.query("SELECT * FROM transfer")
	}
	var args []interface{}
	stmt := "SELECT * FROM transfer WHERE 1"
	condition := "blockNumber"
	if transferFilter.Range != nil {
		if transferFilter.Range.Unit == Time {
			condition = "blockTime"
		}
		args = append(args, transferFilter.Range.From)
		stmt += " AND " + condition + " >= ? "
		if transferFilter.Range.To >= transferFilter.Range.From {
			args = append(args, transferFilter.Range.To)
			stmt += " AND " + condition + " <= ? "
		}
	}
	if transferFilter.TxID != nil {
		args = append(args, transferFilter.TxID.Bytes())
		stmt += " AND txID = ? "
	}
	length := len(transferFilter.AddressSets)
	if length > 0 {
		for i, addressSet := range transferFilter.AddressSets {
			if i == 0 {
				stmt += " AND (( 1 "
			} else {
				stmt += " OR ( 1 "
			}
			if addressSet.TxOrigin != nil {
				args = append(args, addressSet.TxOrigin.Bytes())
				stmt += " AND txOrigin = ? "
			}
			if addressSet.From != nil {
				args = append(args, addressSet.From.Bytes())
				stmt += " AND fromAddress = ? "
			}
			if addressSet.To != nil {
				args = append(args, addressSet.To.Bytes())
				stmt += " AND toAddress = ? "
			}
			if i == length-1 {
				stmt += " )) "
			} else {
				stmt += " ) "
			}
		}
	}
	if transferFilter.Order == DESC {
		stmt += " ORDER BY blockNumber,transferIndex DESC "
	} else {
		stmt += " ORDER BY blockNumber,transferIndex ASC "
	}
	if transferFilter.Options != nil {
		stmt += " limit ?, ? "
		args = append(args, transferFilter.Options.Offset, transferFilter.Options.Limit)
	}
	return db.query(stmt, args...)
}

//query query transfer logs
func (db *TransferDB) query(stmt string, args ...interface{}) ([]*Transfer, error) {
	rows, err := db.db.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var transfers []*Transfer
	for rows.Next() {
		var (
			blockID     []byte
			index       uint32
			blockNumber uint32
			blockTime   uint64
			txID        []byte
			txOrigin    []byte
			from        []byte
			to          []byte
			value       []byte
		)
		if err := rows.Scan(
			&blockID,
			&index,
			&blockNumber,
			&blockTime,
			&txID,
			&txOrigin,
			&from,
			&to,
			&value,
		); err != nil {
			return nil, err
		}
		trans := &Transfer{
			BlockID:     thor.BytesToBytes32(blockID),
			Index:       index,
			BlockNumber: blockNumber,
			BlockTime:   blockTime,
			TxID:        thor.BytesToBytes32(txID),
			TxOrigin:    thor.BytesToAddress(txOrigin),
			From:        thor.BytesToAddress(from),
			To:          thor.BytesToAddress(to),
			Value:       new(big.Int).SetBytes(value),
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
