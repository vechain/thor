package account

import (
	"math/big"

	"github.com/vechain/vecore/cry"

	"github.com/vechain/vecore/acc"
)

// Account manage acc.Account and Storage.
type Account struct {
	Address       acc.Address
	Data          acc.Account
	Storage       acc.Storage
	cachedStorage acc.Storage
	suicided      bool // 标记是否删除
}

func newAccount(addr acc.Address, account acc.Account) *Account {
	return &Account{
		Address:       addr,
		Data:          account,
		Storage:       make(acc.Storage),
		cachedStorage: make(acc.Storage),
		suicided:      false,
	}
}

func (c *Account) deepCopy() *Account {
	data := acc.Account{
		Balance:     new(big.Int).Set(c.Data.Balance),
		CodeHash:    c.Data.CodeHash,
		StorageRoot: c.Data.StorageRoot,
	}
	return &Account{
		Address:       c.Address,
		Data:          data,
		Storage:       c.Storage.Copy(),
		cachedStorage: c.cachedStorage.Copy(),
	}
}

func (c *Account) setBalance(amount *big.Int) {
	c.Data.Balance = amount
}

func (c *Account) getBalance() *big.Int {
	return c.Data.Balance
}

func (c *Account) getCodeHash() cry.Hash {
	return c.Data.CodeHash
}
