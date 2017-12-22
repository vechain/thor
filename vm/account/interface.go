package account

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// StateReader return a accout.
type StateReader interface {
	Get(acc.Address) (*acc.Account, error)
}

// StorageReader return a storage.
type StorageReader interface {
	Get(cry.Hash, cry.Hash) (cry.Hash, error)
}

// KVReader get value from a key.
type KVReader interface {
	Get([]byte) ([]byte, error)
}
