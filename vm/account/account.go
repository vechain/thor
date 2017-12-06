package account

import (
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/vecore/cry"

	"github.com/vechain/vecore/acc"
)

// KVReader get value from a key.
type KVReader interface {
	GetValue(cry.Hash) []byte
}

// Account manage acc.Account and Storage.
type Account struct {
	Address acc.Address
	Data    acc.Account
	Storage acc.Storage
	Code    []byte

	cachedStorage acc.Storage
	suicided      bool // 标记是否删除
}

func newAccount(addr acc.Address, account acc.Account) *Account {
	return &Account{
		Address:       addr,
		Data:          account,
		Storage:       make(acc.Storage),
		Code:          nil,
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

func (c *Account) getCode(kv KVReader) []byte {
	if c.Code != nil {
		return c.Code
	}
	c.Code = kv.GetValue(c.Data.CodeHash)
	return c.Code
}

func (c *Account) setCode(code []byte) {
	c.Code = code
	c.Data.CodeHash = cry.Hash(crypto.Keccak256Hash(code))
}
