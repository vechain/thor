package builder

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

type State interface {
	Error() error

	GetBalance(acc.Address) *big.Int
	GetCode(acc.Address) []byte
	GetStorage(acc.Address, cry.Hash) cry.Hash
	Exists(acc.Address) bool

	SetBalance(acc.Address, *big.Int)
	SetCode(acc.Address, []byte)
	SetStorage(acc.Address, cry.Hash, cry.Hash)
	Delete(acc.Address)

	Commit() cry.Hash
}
