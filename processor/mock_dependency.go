package processor

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// State implement Stater.
type State struct {
	accounts map[acc.Address]*acc.Account
	storages map[cry.Hash]cry.Hash
}

// NewState mock Stater interface.
func NewState() *State {
	state := &State{
		make(map[acc.Address]*acc.Account),
		make(map[cry.Hash]cry.Hash),
	}
	return state
}

// SetOwner mock a rich account.
func (st *State) SetOwner(addr acc.Address) {
	st.accounts[addr] = &acc.Account{
		Balance:     big.NewInt(5000000000000000000),
		CodeHash:    cry.Hash{},
		StorageRoot: cry.Hash{},
	}
}

// GetAccout get account.
func (st *State) GetAccout(addr acc.Address) *acc.Account {
	return st.accounts[addr]
}

// GetStorage get storage.
func (st *State) GetStorage(key cry.Hash) cry.Hash {
	return st.storages[key]
}

// UpdateAccount update memory.
func (st *State) UpdateAccount(addr acc.Address, account *acc.Account) error {
	st.accounts[addr] = account
	return nil
}

// UpdateStorage update memory.
func (st *State) UpdateStorage(key cry.Hash, value cry.Hash) error {
	st.storages[key] = value
	return nil
}
