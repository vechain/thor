package txpool

import (
	"math/big"

	"github.com/vechain/thor/tx"
)

//txObject wrap transaction
type txObject struct {
	tx           *tx.Transaction
	overallGP    *big.Int
	creationTime int64
}

//TxObjects array of TxObject
type txObjects []*txObject

//newTxObject NewTxObject
func newTxObject(tx *tx.Transaction, creationTime int64) *txObject {
	return &txObject{
		tx:           tx,
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
