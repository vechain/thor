package account

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/vechain/vecore/acc"
)

// Account manage acc.Account and Storage.
type Account struct {
	address      acc.Address
	account      acc.Account
	storage      state.Storage
	dirtyStorage state.Storage
	deleted      bool
}

func newAccount(addr acc.Address, acc acc.Account) *Account {
	return &Account{
		address:      addr,
		account:      acc,
		storage:      make(state.Storage),
		dirtyStorage: make(state.Storage),
	}
}

func (c *Account) deepCopy() *Account {
	account := acc.Account{
		Balance:     new(big.Int).Set(c.account.Balance),
		CodeHash:    c.account.CodeHash,
		StorageRoot: c.account.StorageRoot,
	}
	return &Account{
		address:      c.address,
		account:      account,
		storage:      c.storage.Copy(),
		dirtyStorage: c.dirtyStorage.Copy(),
	}
}

func (c *Account) setBalance(amount *big.Int) {
	c.account.Balance = amount
}

func (c *Account) getBalance() *big.Int {
	return c.account.Balance
}

// // addBalance add amount to c's balance.
// // It is used to add funds to the destination account of a transfer.
// // :rtype: bool, if changed return true, or return false
// func (c *Account) addBalance(amount *big.Int) bool {
// 	if amount.Sign() == 0 {
// 		return false
// 	}
// 	newBalance := new(big.Int).Add(c.account.Balance, amount)
// 	c.setBalance(newBalance)
// 	return true
// }
