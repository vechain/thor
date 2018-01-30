package txpool

import (
	"github.com/vechain/thor/tx"
	"math/big"
)

//TxObject wrap transaction
type TxObject struct {
	addTime    int64
	conversion *big.Int
	tx         *tx.Transaction
}

//NewTxObject NewTxObject
func NewTxObject(tx *tx.Transaction, conversion *big.Int, addTime int64) *TxObject {
	return &TxObject{
		addTime,
		conversion,
		tx,
	}
}

//Cost Cost
func (obj *TxObject) Cost() *big.Int {
	en := new(big.Int).Mul(obj.tx.GasPrice(), big.NewInt(int64(obj.tx.Gas())))
	if obj.conversion.Cmp(en) > 0 {
		return en.Mul(en, big.NewInt(2))
	}
	return new(big.Int).Add(en, obj.conversion)
}
