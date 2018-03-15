package txpool

import (
	"github.com/vechain/thor/tx"
	"math/big"
)

//TxObject wrap transaction
type TxObject struct {
	transaction  *tx.Transaction
	overallGP    *big.Int
	creationTime int64
}

//TxObjects array of TxObject
type TxObjects []*TxObject

//NewTxObject NewTxObject
func NewTxObject(transaction *tx.Transaction, creationTime int64) *TxObject {
	return &TxObject{
		transaction:  transaction,
		overallGP:    new(big.Int),
		creationTime: creationTime,
	}
}

//SetOverallGP set overallGP of txObejct
func (obj *TxObject) SetOverallGP(overallGP *big.Int) {
	obj.overallGP = overallGP
}

//CreationTime returns obj's CreationTime
func (obj *TxObject) CreationTime() int64 {
	return obj.creationTime
}

//OverallGP returns obj's overallGP
func (obj *TxObject) OverallGP() *big.Int {
	return obj.overallGP
}

//Transaction returns obj's transaction
func (obj *TxObject) Transaction() *tx.Transaction {
	return obj.transaction
}
