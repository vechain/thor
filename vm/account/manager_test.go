package account

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

type stateReader struct {
}

func (sr *stateReader) GetAccout(acc.Address) acc.Account {
	return acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{1, 2, 3},
		StorageRoot: cry.Hash{1, 2, 3},
	}
}

func (sr *stateReader) GetStorage(cry.Hash) cry.Hash {
	return cry.Hash{1, 2, 3}
}

func TestManager_GetDirtiedAccounts(t *testing.T) {
	assert := assert.New(t)
	manager := NewManager(new(stateReader))

	manager.CreateAccount(acc.Address{1})              // 1 accout
	manager.CreateAccount(acc.Address{2})              // 2 accout
	manager.AddBalance(acc.Address{3}, big.NewInt(10)) // 3 accout and dirty

	dAccounts := manager.GetDirtiedAccounts()
	assert.Equal(len(dAccounts), 1, "应该只有一个 accout 被修改.")
	for _, account := range dAccounts {
		assert.Equal(account.Data.Balance, big.NewInt(10), "dirty accout's balace must be 10.")
	}

	manager.AddBalance(acc.Address{3}, big.NewInt(20)) // 3 accout and dirty
	for _, account := range dAccounts {
		assert.Equal(account.Data.Balance, big.NewInt(30), "dirty accout's balace must be 30.")
	}

	manager.AddBalance(acc.Address{4}, big.NewInt(10)) // 3 accout and dirty
	dAccounts = manager.GetDirtiedAccounts()
	assert.Equal(len(dAccounts), 2, "应该有二个 accout 被修改.")
}
