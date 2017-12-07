package account

import (
	"math/big"

	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

// Manager is account's delegate.
// Implements vm.AccountManager.
type Manager struct {
	kvReader    KVReader
	stateReader StateReader

	// This map holds 'live' objects, which will get modified while processing a state transition.
	accounts      map[acc.Address]*Account // memory cache
	accountsDirty map[acc.Address]struct{} // 用 map 来判断 address 是否被修改过

	// The refund counter, also used by state transitioning.
	refund *big.Int

	preimages map[cry.Hash][]byte
}

// NewManager return a manager for Accounts.
func NewManager(kv KVReader, state StateReader) *Manager {
	return &Manager{
		kvReader:      kv,
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

// CreateAccount create a new accout with empty,
// if the addr already bound to another accout, set it's balance.
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
func (m *Manager) GetBalance(addr acc.Address) *big.Int {
	account := m.getAccount(addr)
	if account == nil {
		return nil
	}
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
	account := m.getAccount(addr)
	if account == nil {
		return cry.Hash{}
	}
	return account.getCodeHash()
}

// GetCode return code from account.
func (m *Manager) GetCode(addr acc.Address) []byte {
	account := m.getAccount(addr)
	if account == nil {
		return nil
	}
	return account.getCode(m.kvReader)
}

// SetCode set account's code.
func (m *Manager) SetCode(addr acc.Address, code []byte) {
	account := m.getOrCreateAccout(addr)
	account.setCode(code)
	m.markAccoutDirty(addr)
}

// GetCodeSize return lenth of account'code.
func (m *Manager) GetCodeSize(addr acc.Address) int {
	account := m.getAccount(addr)
	if account == nil {
		return 0
	}
	code := account.getCode(m.kvReader)
	if code == nil {
		return 0
	}
	return len(code)
}

// AddRefund add gas to refund.
func (m *Manager) AddRefund(gas *big.Int) {
	m.refund.Add(m.refund, gas)
}

// GetRefund returns the current value of the refund counter.
// The return value must not be modified by the caller and will become
// invalid at the next call to AddRefund.
func (m *Manager) GetRefund() *big.Int {
	return m.refund
}

// GetState return storage by given key.
func (m *Manager) GetState(addr acc.Address, key cry.Hash) cry.Hash {
	account := m.getAccount(addr)
	if account == nil {
		return cry.Hash{}
	}
	return account.getStorage(m.stateReader, key)
}

// SetState set storage by given key and value.
func (m *Manager) SetState(addr acc.Address, key cry.Hash, value cry.Hash) {
	account := m.getOrCreateAccout(addr)
	account.setStorage(key, value)
	m.markAccoutDirty(addr)
}

// Suicide kill a account.
func (m *Manager) Suicide(addr acc.Address) {
	account := m.getAccount(addr)
	if account != nil {
		account.suicide()
		m.markAccoutDirty(addr)
	}
}

// HasSuicided return a account or not suicide.
func (m *Manager) HasSuicided(addr acc.Address) bool {
	account := m.getAccount(addr)
	if account != nil {
		return account.hasSuicided()
	}
	return false
}

// Exist reports whether the given account exists in state.
// Notably this should also return true for suicided accounts.
func (m *Manager) Exist(addr acc.Address) bool {
	return m.getAccount(addr) != nil
}

// Empty returns whether the given account is empty.
// Empty is defined according to EIP161 (balance = nonce = code = 0).
func (m *Manager) Empty(addr acc.Address) bool {
	account := m.getAccount(addr)
	return account == nil || account.empty()
}

// ForEachStorage Unrealized.
func (m *Manager) ForEachStorage(addr acc.Address, cb func(cry.Hash, cry.Hash) bool) {
	panic("ForEachStorage Unrealized!")
}

// AddPreimage records a SHA3 preimage seen by the VM.
func (m *Manager) AddPreimage(hash cry.Hash, preimage []byte) {
	if _, ok := m.preimages[hash]; !ok {
		pi := make([]byte, len(preimage))
		copy(pi, preimage)
		m.preimages[hash] = pi
	}
}

// Preimages returns a list of SHA3 preimages that have been submitted.
func (m *Manager) Preimages() map[cry.Hash][]byte {
	return m.preimages
}

// markAccoutDirty mark the account is dirtied.
func (m *Manager) markAccoutDirty(addr acc.Address) {
	_, isDirty := m.accountsDirty[addr]
	if !isDirty {
		m.accountsDirty[addr] = struct{}{}
	}
}

func (m *Manager) getAccount(addr acc.Address) *Account {
	// step 1. get from memory.
	acc := m.accounts[addr]
	if acc != nil {
		if acc.Suicided {
			return nil
		}
		return acc
	}

	// step 2. if acc == nil, then get from stateReader
	account := m.stateReader.GetAccout(addr)
	if account == nil {
		return nil
	}

	newobj := newAccount(addr, m.stateReader.GetAccout(addr))
	m.accounts[addr] = newobj
	return newobj
}

// createAccount create a empty accout.
func (m *Manager) createAccount(addr acc.Address) (*Account, *Account) {
	prev := m.getAccount(addr)
	newobj := newAccount(addr, &acc.Account{})
	m.accounts[addr] = newobj
	return newobj, prev
}

// getOrCreateAccout get a account from memory cache or create a new from StateReader.
func (m *Manager) getOrCreateAccout(addr acc.Address) *Account {
	if acc := m.getAccount(addr); acc != nil {
		return acc
	}

	// If memory and stateReader dot have the given account,
	// then create a new accout with empty.
	account, _ := m.createAccount(addr)
	return account
}
