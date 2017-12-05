package account

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

func TestAccount_deepCopy(t *testing.T) {
	assert := assert.New(t)

	account1 := newAccount(acc.Address{1}, acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{1, 2, 3},
		StorageRoot: cry.Hash{1, 2, 3},
	})
	account1.storage[cry.Hash{1}] = cry.Hash{200}
	account1.dirtyStorage[cry.Hash{1}] = cry.Hash{200}

	account2 := account1.deepCopy()
	assertAccount(assert, account1, account2)
}

func assertAccount(assert *assert.Assertions, account1 *Account, account2 *Account) {
	assert.Equal(account1, account2, "未改值前应该相等.")

	account1.account.Balance.SetInt64(100)
	assert.NotEqual(account1.account.Balance, account2.account.Balance, "修改了 Balance, 应该不相等.")

	account1.account.CodeHash = cry.Hash{1, 2, 3, 1, 2, 3}
	assert.NotEqual(account1.account.CodeHash, account2.account.CodeHash, "修改了 CodeHash, 应该不相等.")

	account1.account.StorageRoot = cry.Hash{1, 2, 3, 1, 2, 3}
	assert.NotEqual(account1.account.StorageRoot, account2.account.StorageRoot, "修改了 StorageRoot, 应该不相等.")

	account1.address = acc.Address{2}
	assert.NotEqual(account1.address, account2.address, "修改了 address, 应该不相等.")

	account1.storage[cry.Hash{1}] = cry.Hash{100}
	assert.NotEqual(account1.storage, account2.storage, "修改了 storage, 应该不相等.")

	account1.dirtyStorage[cry.Hash{1}] = cry.Hash{100}
	assert.NotEqual(account1.dirtyStorage, account2.dirtyStorage, "修改了 dirtyStorage, 应该不相等.")
}
