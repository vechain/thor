package account

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

type stateReader struct {
}

func (st *stateReader) Exist(addr acc.Address) bool {
	return true
}

func (st *stateReader) GetStorage(acc.Address, cry.Hash) cry.Hash {
	return cry.Hash{1, 2, 3}
}

func (st *stateReader) GetBalance(addr acc.Address) *big.Int {
	return new(big.Int)
}

func (st *stateReader) GetCode(addr acc.Address) []byte {
	return []byte{1}
}

func TestManager_GetDirtyAccounts(t *testing.T) {
	assert := assert.New(t)
	manager := NewManager(new(stateReader))

	manager.CreateAccount(acc.Address{1})              // 1 accout, stateReader will certainly return a account, so dirty +1
	manager.CreateAccount(acc.Address{2})              // 2 accout, stateReader will certainly return a account, so dirty +1
	manager.AddBalance(acc.Address{3}, big.NewInt(10)) // 3 accout and dirty

	dAccounts := manager.GetDirtyAccounts()
	assert.Equal(len(dAccounts), 3, "应该三个 accout 被修改.")

	for _, account := range dAccounts {
		if account.Address == (acc.Address{1}) {
			assert.Equal(account.Balance, new(big.Int), "dirty accout's balace must be 0.")
		} else if account.Address == (acc.Address{3}) {
			assert.Equal(account.Balance, big.NewInt(10), "dirty accout's balace must be 10.")
		}

	}

	manager.AddBalance(acc.Address{3}, big.NewInt(20)) // 3 accout and dirty
	dAccounts = manager.GetDirtyAccounts()
	assert.Equal(len(dAccounts), 3, "应该三个 accout 被修改.")

	for _, account := range dAccounts {
		if account.Address == (acc.Address{3}) {
			assert.Equal(account.Balance, big.NewInt(30), "dirty accout's balace must be 30.")
		}
	}

	manager.AddBalance(acc.Address{4}, big.NewInt(10)) // 4 accout and dirty
	dAccounts = manager.GetDirtyAccounts()
	assert.Equal(len(dAccounts), 4, "应该四个 accout 被修改.")
}

func TestManager_GetStorage(t *testing.T) {
	assert := assert.New(t)
	manager := NewManager(new(stateReader))
	addr := acc.Address{1}

	right := manager.GetState(addr, cry.Hash{})
	assert.Equal(right, cry.Hash{1, 2, 3})
}

func TestManager_SetCode(t *testing.T) {
	assert := assert.New(t)
	manager := NewManager(new(stateReader))
	addr := acc.Address{1}
	code := []byte{4, 5, 6}
	codeHash := cry.Hash(crypto.Keccak256Hash(code))

	manager.SetCode(addr, code)

	assert.Equal(manager.GetCode(addr), code)
	assert.Equal(manager.GetCodeHash(addr), codeHash)
}

type emptyStateReader struct {
	stateReader
}

func (st *emptyStateReader) GetCode(addr acc.Address) []byte {
	return nil
}

func TestManager_Empty(t *testing.T) {
	assert := assert.New(t)
	emptyManager := NewManager(new(emptyStateReader))
	assert.Equal(emptyManager.Empty(acc.Address{1}), true)

	manager := NewManager(new(stateReader))
	assert.Equal(manager.Empty(acc.Address{1}), false)
}
