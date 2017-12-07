package account

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// StateReader return a accout or storage.
type StateReader interface {
	GetAccout(acc.Address) *acc.Account // if don't have, return nil
	GetStorage(cry.Hash) cry.Hash
}

// KVReader get value from a key.
type KVReader interface {
	GetValue(cry.Hash) []byte // if don't have, return nil
}
