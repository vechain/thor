package account

import (
	"math/big"

	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

// StateReader return a accout or storage.
type StateReader interface {
	GetAccout(acc.Address) acc.Account
	GetStorage(cry.Hash) cry.Hash
}

// Manager is account's delegate.
// Implements vm.AccountManager.
type Manager struct {
	stateReader StateReader

	// This map holds 'live' objects, which will get modified while processing a state transition.
	accounts      map[acc.Address]*Account // memory cache
	accountsDirty map[acc.Address]struct{} // 用 map 来判断 address 是否被修改过

	// The refund counter, also used by state transitioning.
	refund *big.Int

	preimages map[cry.Hash][]byte
}

// NewManager return a manager for Accounts.
func NewManager(state StateReader) *Manager {
	return &Manager{
		stateReader:   state,
		accounts:      make(map[acc.Address]*Account),
		accountsDirty: make(map[acc.Address]struct{}),
		refund:        new(big.Int),
		preimages:     make(map[cry.Hash][]byte),
	}
}

// DeepCopy Full backup of the current status, future changes will not affect them.
func (m *Manager) DeepCopy() interface{} {
	accounts := make(map[acc.Address]*Account)
	for key, value := range m.accounts {
		accounts[key] = value.deepCopy()
	}

	accountsDirty := make(map[acc.Address]struct{})
	for key := range m.accountsDirty {
		accountsDirty[key] = struct{}{}
	}

	preimages := make(map[cry.Hash][]byte)
	for key, value := range m.preimages {
		preimages[key] = make([]byte, len(value))
		copy(preimages[key], value)
	}

	return &Manager{
		stateReader:   m.stateReader,
		accounts:      accounts,
		accountsDirty: accountsDirty,
		refund:        new(big.Int).Set(m.refund),
		preimages:     preimages,
	}
}

// markAccoutDirty mark the account is dirtied.
func (m *Manager) markAccoutDirty(addr acc.Address) {
	_, isDirty := m.accountsDirty[addr]
	if !isDirty {
		m.accountsDirty[addr] = struct{}{}
	}
}

// GetAccout get a account from memory cache or IStateReader.
func (m *Manager) getAccout(addr acc.Address) *Account {
	if acc := m.accounts[addr]; acc != nil {
		if acc.suicided {
			return nil
		}
		return acc
	}
	account := newAccount(addr, m.stateReader.GetAccout(addr))
	m.accounts[addr] = account
	return account
}

// AddBalance adds amount to the account associated with addr
func (m *Manager) AddBalance(addr acc.Address, amount *big.Int) {
	if account := m.getAccout(addr); account != nil {
		if amount.Sign() == 0 {
			return
		}
		account.setBalance(new(big.Int).Add(account.getBalance(), amount))
	}
}

// GetDirtiedAccounts return all dirtied Accounts.
func (m *Manager) GetDirtiedAccounts() []*Account {
	var dirtyAccounts = make([]*Account, len(m.accountsDirty))
	for addr := range m.accountsDirty {
		account := m.accounts[addr]
		dirtyAccounts = append(dirtyAccounts, account)
	}
	return dirtyAccounts
}

// // Preimages returns a list of SHA3 preimages that have been submitted.
// func (m *Manager) Preimages() map[cry.Hash][]byte {
// 	return m.preimages
// }

// // AddRefund add Refund.
// func (m *Manager) AddRefund(gas *big.Int) {
// 	m.refund.Add(m.refund, gas)
// }

// // GetRefund returns the current value of the refund counter.
// // The return value must not be modified by the caller and will become
// // invalid at the next call to AddRefund.
// func (m *Manager) GetRefund() *big.Int {
// 	return m.refund
// }
