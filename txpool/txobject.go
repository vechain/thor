package txpool

import (
	"math/big"

	"github.com/vechain/thor/tx"
)

type ObjectStatus int

const (
	Pending ObjectStatus = iota
	Queued
)

//txObject wrap transaction
type txObject struct {
	tx           *tx.Transaction
	status       ObjectStatus
	overallGP    *big.Int
	creationTime int64
}

//TxObjects array of TxObject
type txObjects []*txObject

//newTxObject NewTxObject
func newTxObject(tx *tx.Transaction, creationTime int64, status ObjectStatus) *txObject {
	return &txObject{
		tx:           tx,
		status:       status,
		overallGP:    new(big.Int),
		creationTime: creationTime,
	}
}

//SetOverallGP set overallGP of txObejct
func (obj *txObject) SetOverallGP(overallGP *big.Int) {
	obj.overallGP = overallGP
}

//CreationTime returns obj's CreationTime
func (obj *txObject) CreationTime() int64 {
	return obj.creationTime
}

//OverallGP returns obj's overallGP
func (obj *txObject) OverallGP() *big.Int {
	return obj.overallGP
}

//Tx returns obj's transaction
func (obj *txObject) Tx() *tx.Transaction {
	return obj.tx
}

func (obj *txObject) Status() ObjectStatus {
	return obj.status
}

func (obj *txObject) SetStatus(status ObjectStatus) {
	obj.status = status
}
