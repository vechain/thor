package account

import (
	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

// StateReader return a accout or storage.
type StateReader interface {
	GetAccout(acc.Address) *acc.Account
	GetStorage(cry.Hash) cry.Hash
}

// KVReader get value from a key.
type KVReader interface {
	GetValue(cry.Hash) []byte
}
