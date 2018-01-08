package runtime

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

type State interface {
	Error() error
	GetBalance(acc.Address) *big.Int
	GetCode(acc.Address) []byte
	GetCodeHash(acc.Address) cry.Hash
	GetStorage(acc.Address, cry.Hash) cry.Hash
	Exists(acc.Address) bool
	ForEachStorage(addr acc.Address, cb func(key, value cry.Hash) bool)

	SetBalance(acc.Address, *big.Int)
	SetCode(acc.Address, []byte)
	SetStorage(acc.Address, cry.Hash, cry.Hash)
	Delete(acc.Address)

	NewCheckpoint() int
	Revert()
}
