package state

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// StorageKey is composite keys for storage.
type StorageKey struct {
	Addr acc.Address
	Key  cry.Hash
}

// Account manage acc.Account and Storage.
type Account struct {
	Balance  *big.Int
	Code     []byte
	Suicided bool
}
