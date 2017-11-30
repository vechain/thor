package account

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/vecore/acc"
)

// IStateReader return a accout or storage.
type IStateReader interface {
	GetAccout(acc.Address) acc.Account
	GetStorage(common.Hash) common.Hash
}

// Manager is account's delegate.
// Implements vm.AccountManager
type Manager struct {
	stateReader IStateReader

	// This map holds 'live' objects, which will get modified while processing a state transition.
	accounts      map[acc.Address]*Account // memory cache
	accountsDirty map[acc.Address]struct{} // 用 map 来判断 address 是否被修改过

	// The refund counter, also used by state transitioning.
	refund *big.Int

	preimages map[common.Hash][]byte

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal journal
}

// NewManager return a manager for Accounts.
func NewManager(state IStateReader) *Manager {
	return &Manager{
		stateReader:   state,
		accounts:      make(map[acc.Address]*Account),
		accountsDirty: make(map[acc.Address]struct{}),
		refund:        new(big.Int),
		preimages:     make(map[common.Hash][]byte),
	}
}

// GetAccout get a account from memory cache or IStateReader.
func (m *Manager) getAccout(addr acc.Address) *Account {
	if obj := m.accounts[addr]; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}
	account := newAccount(addr, m.stateReader.GetAccout(addr))
	m.accounts[addr] = account
	return account
}

// SetBalance set amount to the account associated with addr
func (m *Manager) SetBalance(addr acc.Address, amount *big.Int) {
	if account := m.getAccout(addr); account != nil {
		account.setBalance(amount)
		m.accountsDirty[addr] = struct{}{}
	}
}

// AddBalance adds amount to the account associated with addr
func (m *Manager) AddBalance(addr acc.Address, amount *big.Int) {
	if account := m.getAccout(addr); account != nil {
		if amount.Sign() == 0 {
			return
		}
		balance := account.getBalance()
		account.setBalance(new(big.Int).Add(balance, amount))
		m.journal = append(m.journal, balanceChange{
			&addr,
			new(big.Int).Set(balance),
		})
		m.accountsDirty[addr] = struct{}{}
	}
}

// // Preimages returns a list of SHA3 preimages that have been submitted.
// func (m *Manager) Preimages() map[common.Hash][]byte {
// 	return m.preimages
// }

// // AddRefund add Refund.
// func (m *Manager) AddRefund(gas *big.Int) {
// 	m.journal = append(m.journal, refundChange{prev: new(big.Int).Set(m.refund)})
// 	m.refund.Add(m.refund, gas)
// }

// // GetRefund returns the current value of the refund counter.
// // The return value must not be modified by the caller and will become
// // invalid at the next call to AddRefund.
// func (m *Manager) GetRefund() *big.Int {
// 	return m.refund
// }
