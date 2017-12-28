package state_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	. "github.com/vechain/thor/vm/state"
)

// Reader stub.
type reader struct {
}

func (r reader) GetBalance(acc.Address) *big.Int { return nil }

func (r reader) GetCode(acc.Address) []byte { return nil }

func (r reader) GetStorage(acc.Address, cry.Hash) cry.Hash { return cry.Hash{} }

func (r reader) Exists(acc.Address) bool { return false }

func TestStateSnapshot(t *testing.T) {
	assert := assert.New(t)
	state := New(reader{})

	addrOne := acc.Address{1}
	addrTwo := acc.Address{2}
	stKey := StorageKey{
		Addr: addrOne,
		Key:  cry.Hash{10}}

	////
	ver := state.Snapshot()
	assert.Equal(ver, 0)

	state.AddBalance(common.Address(addrOne), big.NewInt(10))
	state.AddBalance(common.Address(addrTwo), big.NewInt(20))
	state.SetState(common.Address(addrOne), common.Hash{10}, common.Hash{10})
	accMap, storages := state.GetAccountAndStorage()
	assert.Equal(accMap[addrOne].Balance, big.NewInt(10))
	assert.Equal(accMap[addrTwo].Balance, big.NewInt(20))
	assert.Equal(storages[stKey], cry.Hash{10})

	////
	ver = state.Snapshot()
	assert.Equal(ver, 1)

	state.AddBalance(common.Address(addrOne), big.NewInt(20))
	state.AddBalance(common.Address(addrTwo), big.NewInt(30))
	state.SetState(common.Address(addrOne), common.Hash{10}, common.Hash{20})
	accMap, storages = state.GetAccountAndStorage()
	assert.Equal(accMap[addrOne].Balance, big.NewInt(30))
	assert.Equal(accMap[addrTwo].Balance, big.NewInt(50))
	assert.Equal(storages[stKey], cry.Hash{20})

	////
	state.RevertToSnapshot(ver)

	accMap, storages = state.GetAccountAndStorage()
	assert.Equal(accMap[addrOne].Balance, big.NewInt(10))
	assert.Equal(accMap[addrTwo].Balance, big.NewInt(20))
	assert.Equal(storages[stKey], cry.Hash{10})

	////
	ver = state.Snapshot()
	assert.Equal(ver, 1)

	state.AddBalance(common.Address(addrOne), big.NewInt(20))
	accMap, _ = state.GetAccountAndStorage()
	assert.Equal(accMap[addrOne].Balance, big.NewInt(30))
	assert.Equal(accMap[addrTwo].Balance, big.NewInt(20))
}
