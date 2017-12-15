package processor

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// Stater can reade|update account&storage.
type Stater interface {
	GetAccout(acc.Address) *acc.Account // if don't have, return nil
	GetStorage(cry.Hash) cry.Hash
	UpdateAccount(acc.Address, *acc.Account) error
	UpdateStorage(cry.Hash, cry.Hash) error
}
