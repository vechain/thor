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

func TestManager_DeepCopy(t *testing.T) {
	assert := assert.New(t)
	addr := acc.Address{1}
	manager1 := NewManager(new(stateReader))
	manager1.AddBalance(addr, big.NewInt(10))
	manager2 := manager1.DeepCopy()

	// test for Manager.Account
	assertAccount(assert, manager1.getAccout(addr), manager2.getAccout(addr))
}
