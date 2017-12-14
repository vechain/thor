package processor

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// Messager is  a unit which can be handle.
type Messager interface {
	To() *acc.Address
	Value() *big.Int
	Data() []byte

	// returns hash of tx which generated this message.
	TransactionHash() cry.Hash
}

// Transactioner is the aggregation of Messages.
type Transactioner interface {
	From() acc.Address
	AsMessages() ([]Messager, error)
	GasPrice() *big.Int
	GasLimit() *big.Int
}

// State can reade|update account&storage.
type State interface {
	GetAccout(acc.Address) *acc.Account // if don't have, return nil
	GetStorage(cry.Hash) cry.Hash
	UpdateAccount(acc.Address, *acc.Account) error
	UpdateStorage(cry.Hash, cry.Hash) error
}
