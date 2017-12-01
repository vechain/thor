package account

import (
	"math/big"
	"testing"

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
	// assert := assert.New(t)
	// sr := new(stateReader)

	// manager1 := NewManager(sr)
	// manager1.AddBalance(acc.Address{1}, big.NewInt(10))

	// manager2 := manager1.DeepCopy()
	// manager1.getAccout(acc.Address{1}).account.Balance.SetInt64(100)
	// manager1.getAccout(acc.Address{1}).account.CodeHash = cry.Hash{1, 2, 3, 1, 2, 3}
}
