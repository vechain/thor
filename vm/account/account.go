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
	Data         *acc.Account
	DirtyStorage acc.Storage // only dirty storage
	DirtyCode    []byte      // dirty code
	Suicided     bool

	cachedStorage acc.Storage
	cachedCode    []byte
}

func newAccount(addr acc.Address, account *acc.Account) *Account {
	return &Account{
		Address:       addr,
		Data:          account,
		DirtyStorage:  make(acc.Storage),
		cachedStorage: make(acc.Storage),
		DirtyCode:     nil,
		cachedCode:    nil,
		Suicided:      false,
	}
}

func (c *Account) deepCopy() *Account {
	data := &acc.Account{
		Balance:     new(big.Int).Set(c.Data.Balance),
		CodeHash:    c.Data.CodeHash,
		StorageRoot: c.Data.StorageRoot,
	}

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
		Data:          data,
		DirtyStorage:  c.DirtyStorage.Copy(),
		cachedStorage: c.cachedStorage.Copy(),
		DirtyCode:     dirtyCode,
		cachedCode:    cachedCode,
		Suicided:      c.Suicided,
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
	if c.DirtyCode != nil {
		return c.DirtyCode
	}
	c.cachedCode, _ = kv.Get(c.Data.CodeHash[:])
	return c.cachedCode
}

func (c *Account) setCode(code []byte) {
	c.DirtyCode = code
	c.cachedCode = code
	c.Data.CodeHash = cry.Hash(crypto.Keccak256Hash(code))
}

func (c *Account) getStorage(sr StorageReader, key cry.Hash) cry.Hash {
	storage := c.cachedStorage[key]
	if storage != (cry.Hash{0}) {
		return storage
	}
	storage, _ = sr.Get(c.Data.StorageRoot, key)
	c.cachedStorage[key] = storage
	return storage
}

func (c *Account) setStorage(key cry.Hash, value cry.Hash) {
	c.cachedStorage[key] = value
	c.DirtyStorage[key] = value
}

func (c *Account) suicide() {
	c.Data.Balance = new(big.Int)
	c.Suicided = true
}

func (c *Account) hasSuicided() bool {
	return c.Suicided
}

func (c *Account) empty() bool {
	return c.Data.Balance.Sign() == 0 && c.Data.CodeHash == cry.Hash{}
}
