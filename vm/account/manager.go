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

// GetDirtiedAccounts return all dirtied Accounts.
func (m *Manager) GetDirtiedAccounts() []*Account {
	var dirtyAccounts = make([]*Account, 0, len(m.accountsDirty))
	for addr := range m.accountsDirty {
		dirtyAccounts = append(dirtyAccounts, m.accounts[addr])
	}
	return dirtyAccounts
}

// CreateAccount create a new accout, if the addr already bound to another accout, set it's balance.
func (m *Manager) CreateAccount(addr acc.Address) {
	new, prev := m.createAccount(addr)
	if prev != nil {
		new.setBalance(prev.Data.Balance)
		m.markAccoutDirty(addr)
	}
}

// AddBalance add amount to the account's balance.
func (m *Manager) AddBalance(addr acc.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	account := m.getOrCreateAccout(addr)
	account.setBalance(new(big.Int).Add(account.getBalance(), amount))
	m.markAccoutDirty(addr)
}

// SubBalance sub amount from the account's balance.
func (m *Manager) SubBalance(addr acc.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	account := m.getOrCreateAccout(addr)
	account.setBalance(new(big.Int).Sub(account.getBalance(), amount))
	m.markAccoutDirty(addr)
}

// GetBalance return the account's balance.
// If the
func (m *Manager) GetBalance(addr acc.Address) *big.Int {
	account := m.getOrCreateAccout(addr)
	return account.getBalance()
}

// GetNonce stub func.
func (m *Manager) GetNonce(acc.Address) uint64 {
	return 0
}

// SetNonce stub func.
func (m *Manager) SetNonce(acc.Address, uint64) {
}

// GetCodeHash return account's CodeHash.
func (m *Manager) GetCodeHash(addr acc.Address) cry.Hash {
	return m.getOrCreateAccout(addr).getCodeHash()
}

func (m *Manager) GetCode(acc.Address) []byte {
	return []byte{}
}

func (m *Manager) SetCode(acc.Address, []byte) {
}

func (m *Manager) GetCodeSize(acc.Address) int {
	return 0
}

func (m *Manager) AddRefund(*big.Int) {
}

func (m *Manager) GetRefund() *big.Int {
	return nil
}

func (m *Manager) GetState(acc.Address, cry.Hash) cry.Hash {
	return cry.Hash{}
}

func (m *Manager) SetState(acc.Address, cry.Hash, cry.Hash) {
}

func (m *Manager) Suicide(acc.Address) bool {
	return false
}

func (m *Manager) HasSuicided(acc.Address) bool {
	return false
}

// Exist reports whether the given account exists in state.
// Notably this should also return true for suicided accounts.
func (m *Manager) Exist(acc.Address) bool {
	return false
}

// Empty returns whether the given account is empty. Empty
// is defined according to EIP161 (balance = nonce = code = 0).
func (m *Manager) Empty(acc.Address) bool {
	return false
}

func (m *Manager) ForEachStorage(acc.Address, func(cry.Hash, cry.Hash) bool) {
}

// AddPreimage records a SHA3 preimage seen by the VM.
func (m *Manager) AddPreimage(hash cry.Hash, preimage []byte) {

}

// markAccoutDirty mark the account is dirtied.
func (m *Manager) markAccoutDirty(addr acc.Address) {
	_, isDirty := m.accountsDirty[addr]
	if !isDirty {
		m.accountsDirty[addr] = struct{}{}
	}
}

func (m *Manager) getAccount(addr acc.Address) *Account {
	acc := m.accounts[addr]
	if acc != nil && acc.suicided {
		return nil
	}
	return acc
}

func (m *Manager) createAccount(addr acc.Address) (*Account, *Account) {
	prev := m.getAccount(addr)
	newobj := newAccount(addr, m.stateReader.GetAccout(addr))
	m.accounts[addr] = newobj
	return newobj, prev
}

// getOrCreateAccout get a account from memory cache or create a new from StateReader.
func (m *Manager) getOrCreateAccout(addr acc.Address) *Account {
	if acc := m.getAccount(addr); acc != nil {
		return acc
	}
	account, _ := m.createAccount(addr)
	return account
}
