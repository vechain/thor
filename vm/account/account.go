package account

import (
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// Account manage acc.Account and Storage.
type Account struct {
	Address      acc.Address
	Balance      *big.Int
	DirtyStorage acc.Storage // only dirty storage
	DirtyCode    []byte      // dirty code
	Suicided     bool

	cachedStorage acc.Storage
	cachedCode    []byte
}

func newAccount(addr acc.Address, balacne *big.Int) *Account {
	return &Account{
		Address:       addr,
		Balance:       balacne,
		DirtyStorage:  make(acc.Storage),
		cachedStorage: make(acc.Storage),
		DirtyCode:     nil,
		cachedCode:    nil,
		Suicided:      false,
	}
}

func (c *Account) deepCopy() *Account {
	var dirtyCode []byte
	var cachedCode []byte

	if c.DirtyCode != nil {
		dirtyCode := make([]byte, len(c.DirtyCode))
		copy(dirtyCode, c.DirtyCode)
	}

	if c.cachedCode != nil {
		cachedCode := make([]byte, len(c.cachedCode))
		copy(cachedCode, c.cachedCode)
	}

	return &Account{
		Address:       c.Address,
		Balance:       new(big.Int).Set(c.Balance),
		DirtyStorage:  c.DirtyStorage.Copy(),
		cachedStorage: c.cachedStorage.Copy(),
		DirtyCode:     dirtyCode,
		cachedCode:    cachedCode,
		Suicided:      c.Suicided,
	}
}

func (c *Account) setBalance(amount *big.Int) {
	c.Balance = amount
}

func (c *Account) getBalance() *big.Int {
	return c.Balance
}

func (c *Account) getCodeHash(state StateReader) cry.Hash {
	code := c.getCode(state)
	return cry.Hash(crypto.Keccak256Hash(code))
}

func (c *Account) getCode(state StateReader) []byte {
	if c.DirtyCode != nil {
		return c.DirtyCode
	}
	c.cachedCode = state.GetCode(c.Address)
	return c.cachedCode
}

func (c *Account) setCode(code []byte) {
	c.DirtyCode = code
	c.cachedCode = code
}

func (c *Account) getStorage(state StateReader, key cry.Hash) cry.Hash {
	storage := c.cachedStorage[key]
	if storage != (cry.Hash{0}) {
		return storage
	}
	storage = state.GetStorage(c.Address, key)
	c.cachedStorage[key] = storage
	return storage
}

func (c *Account) setStorage(key cry.Hash, value cry.Hash) {
	c.cachedStorage[key] = value
	c.DirtyStorage[key] = value
}

func (c *Account) suicide() {
	c.Balance = new(big.Int)
	c.Suicided = true
}

func (c *Account) hasSuicided() bool {
	return c.Suicided
}

func (c *Account) empty(state StateReader) bool {
	return c.Balance.Sign() == 0 && c.getCodeHash(state) == cry.Hash(crypto.Keccak256Hash(nil))
}
