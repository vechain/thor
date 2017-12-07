package account

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

type stateReader struct {
}

func (sr *stateReader) GetAccout(acc.Address) *acc.Account {
	return &acc.Account{
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
	manager := NewManager(nil, new(stateReader))

	manager.CreateAccount(acc.Address{1})              // 1 accout, stateReader will certainly return a account, so dirty +1
	manager.CreateAccount(acc.Address{2})              // 2 accout, stateReader will certainly return a account, so dirty +1
	manager.AddBalance(acc.Address{3}, big.NewInt(10)) // 3 accout and dirty

	dAccounts := manager.GetDirtiedAccounts()
	assert.Equal(len(dAccounts), 3, "应该三个 accout 被修改.")

	for _, account := range dAccounts {
		if account.Address == (acc.Address{1}) {
			assert.Equal(account.Data.Balance, new(big.Int), "dirty accout's balace must be 0.")
		} else if account.Address == (acc.Address{3}) {
			assert.Equal(account.Data.Balance, big.NewInt(10), "dirty accout's balace must be 10.")
		}

	}

	manager.AddBalance(acc.Address{3}, big.NewInt(20)) // 3 accout and dirty
	dAccounts = manager.GetDirtiedAccounts()
	assert.Equal(len(dAccounts), 3, "应该三个 accout 被修改.")

	for _, account := range dAccounts {
		if account.Address == (acc.Address{3}) {
			assert.Equal(account.Data.Balance, big.NewInt(30), "dirty accout's balace must be 30.")
		}
	}

	manager.AddBalance(acc.Address{4}, big.NewInt(10)) // 4 accout and dirty
	dAccounts = manager.GetDirtiedAccounts()
	assert.Equal(len(dAccounts), 4, "应该四个 accout 被修改.")
}

func TestManager_GetCodeHash(t *testing.T) {
	assert := assert.New(t)
	manager := NewManager(nil, new(stateReader))
	addr := acc.Address{1}

	right := manager.GetCodeHash(addr)
	left := cry.Hash{1, 2, 3}
	assert.Equal(right, left)
}

func TestManager_SetCode(t *testing.T) {
	assert := assert.New(t)
	manager := NewManager(nil, new(stateReader))
	addr := acc.Address{1}
	code := []byte{4, 5, 6}
	codeHash := cry.Hash(crypto.Keccak256Hash(code))

	manager.SetCode(addr, code)

	assert.Equal(manager.GetCode(addr), code)
	assert.Equal(manager.GetCodeHash(addr), codeHash)
}

type testKV struct{}

func (kv testKV) GetValue(cry.Hash) []byte {
	return nil
}

type testKV2 struct{}

func (kv testKV2) GetValue(cry.Hash) []byte {
	return []byte{1, 2}
}

func TestManager_GetCodeSize(t *testing.T) {
	assert := assert.New(t)
	manager := NewManager(new(testKV), new(stateReader))
	addr := acc.Address{1}

	assert.Equal(manager.GetCodeSize(addr), 0)

	manager = NewManager(new(testKV2), new(stateReader))
	assert.Equal(manager.GetCodeSize(addr), 2)
}

type emptyStateReader struct {
	stateReader
}

func (sr *emptyStateReader) GetAccout(acc.Address) *acc.Account {
	return nil
}

func TestManager_Empty(t *testing.T) {
	assert := assert.New(t)
	emptyManager := NewManager(nil, new(emptyStateReader))
	assert.Equal(emptyManager.Empty(acc.Address{1}), true)

	manager := NewManager(nil, new(stateReader))
	assert.Equal(manager.Empty(acc.Address{1}), false)
}
