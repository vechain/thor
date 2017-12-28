package state

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// Reader fake.
type Reader interface {
	GetBalance(acc.Address) *big.Int
	GetCode(acc.Address) []byte
	GetStorage(acc.Address, cry.Hash) cry.Hash
	Exists(acc.Address) bool
}
