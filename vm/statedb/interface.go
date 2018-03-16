package statedb

import (
	"math/big"

	"github.com/vechain/thor/thor"
)

// State is defined to decouple with state.State.
type State interface {
	GetBalance(thor.Address) *big.Int
	GetCode(thor.Address) []byte
	GetCodeHash(thor.Address) thor.Hash
	GetStorage(thor.Address, thor.Hash) thor.Hash
	Exists(thor.Address) bool
	ForEachStorage(addr thor.Address, cb func(key thor.Hash, value []byte) bool)

	SetBalance(thor.Address, *big.Int)
	SetCode(thor.Address, []byte)
	SetStorage(thor.Address, thor.Hash, thor.Hash)
	Delete(thor.Address)

	NewCheckpoint() int
	RevertTo(int)
}
