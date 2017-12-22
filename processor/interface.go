package processor

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// Stater can reade|update account.
type Stater interface {
	GetAccout(acc.Address) *acc.Account // if don't have, return nil
	UpdateAccount(acc.Address, *acc.Account) error
	Delete(key []byte) error
}

// Storager can reade|update storage.
type Storager interface {
	GetStorage(cry.Hash) cry.Hash
	UpdateStorage(root cry.Hash, key cry.Hash, value cry.Hash) error
	Hash(root cry.Hash) cry.Hash
}

// KVer get / put value from a key.
type KVer interface {
	GetValue(cry.Hash) []byte // if don't have, return nil
	Put(key, value []byte) error
}
