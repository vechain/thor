package account

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// StateReader fake.
type StateReader interface {
	GetBalance(acc.Address) *big.Int
	GetCode(acc.Address) []byte
	GetStorage(acc.Address, cry.Hash) cry.Hash
	Exist(acc.Address) bool
}
