package state

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/stackedmap"
)

// State is facade for account.Manager, snapshot.Snapshot and Log.
// It implements evm.StateDB, only adapt to evm.
type State struct {
	repo *stackedmap.StackedMap
}

// New is State's factory.
func New(stater Reader) *State {
	getter := func(hash interface{}) (interface{}, bool) {
		switch v := hash.(type) {
		case common.Address:
			if !stater.Exist(acc.Address(v)) {
				return nil, false
			}
			return Account{
				Balance: stater.GetBalance(acc.Address(v)),
				Code:    stater.GetCode(acc.Address(v))}, true
		case StorageKey:
			return common.Hash(stater.GetStorage(v.Addr, v.Key)), true
		}
		return nil, false
	}

	return &State{
		repo: stackedmap.New(getter),
	}
}

func (s *State) getAccount(addr common.Address) (Account, bool) {
	if account, exist := s.repo.Get(addr); exist {
		return account.(Account), true
	}
	return Account{}, false
}

// GetAccountAndStorage return all the dirty.
func (s *State) GetAccountAndStorage() (map[acc.Address]Account, map[StorageKey]cry.Hash) {
	accounts := make(map[acc.Address]Account)
	storage := make(map[StorageKey]cry.Hash)
	for key, value := range s.repo.Puts() {
		switch v := key.(type) {
		case common.Address:
			accounts[acc.Address(v)] = value.(Account)
		case StorageKey:
			storage[v] = cry.Hash(value.(common.Hash))
		}
	}
	return accounts, storage
}

// GetRefund stub.
func (s *State) GetRefund() *big.Int {
	if refund, exist := s.repo.Get("refund"); exist {
		return refund.(*big.Int)
	}
	return big.NewInt(0)
}

// GetPreimages returns a list of SHA3 preimages that have been submitted.
func (s *State) GetPreimages() map[cry.Hash][]byte {
	preimages := make(map[cry.Hash][]byte)
	for key, value := range s.repo.Puts() {
		switch v := key.(type) {
		case common.Hash:
			preimages[cry.Hash(v)] = value.([]byte)
		}
	}
	return preimages
}

// Preimages only compliant for evm.
func (s *State) Preimages() map[common.Hash][]byte {
	preimages := make(map[common.Hash][]byte)
	for key, value := range s.repo.Puts() {
		switch v := key.(type) {
		case common.Hash:
			preimages[v] = value.([]byte)
		}
	}
	return preimages
}

// GetLogs return the log for current state.
func (s *State) GetLogs() []*types.Log {
	if logs, exist := s.repo.Get("logs"); exist {
		return logs.([]*types.Log)
	}
	return nil
}

// ForEachStorage stub.
func (s *State) ForEachStorage(addr common.Address, cb func(common.Hash, common.Hash) bool) {
	panic("ForEachStorage Unrealized!")
}

// GetBalance stub.
func (s *State) GetBalance(addr common.Address) *big.Int {
	if account, exist := s.getAccount(addr); exist {
		return account.Balance
	}
	return big.NewInt(0)
}

// GetNonce stub.
func (s *State) GetNonce(addr common.Address) uint64 {
	return 0
}

// GetCodeHash stub.
func (s *State) GetCodeHash(addr common.Address) common.Hash {
	if account, exist := s.getAccount(addr); exist {
		return crypto.Keccak256Hash(account.Code)
	}
	return common.Hash{}
}

// GetCode stub.
func (s *State) GetCode(addr common.Address) []byte {
	if account, exist := s.getAccount(addr); exist {
		return account.Code
	}
	return nil
}

// GetCodeSize stub.
func (s *State) GetCodeSize(addr common.Address) int {
	if account, exist := s.getAccount(addr); exist {
		return len(account.Code)
	}
	return 0
}

// HasSuicided stub.
func (s *State) HasSuicided(addr common.Address) bool {
	if account, exist := s.getAccount(addr); exist {
		return account.Suicided
	}
	return false
}

// Empty stub.
func (s *State) Empty(addr common.Address) bool {
	if account, exist := s.getAccount(addr); exist {
		return account.Balance.Sign() == 0 && account.Code == nil
	}
	return true
}

// GetState stub.
func (s *State) GetState(addr common.Address, key common.Hash) common.Hash {
	storage, _ := s.repo.Get(StorageKey{acc.Address(addr), cry.Hash(key)})
	return storage.(common.Hash)
}

// Exist stub.
func (s *State) Exist(addr common.Address) bool {
	_, exist := s.repo.Get(addr)
	return exist
}

func (s *State) getOrCreateAccount(addr common.Address) Account {
	if account, exist := s.getAccount(addr); exist {
		return account
	}
	return Account{
		Balance: big.NewInt(0),
		Code:    nil}
}

// CreateAccount stub.
func (s *State) CreateAccount(addr common.Address) {
	new := Account{
		Balance: big.NewInt(0),
		Code:    nil}

	if prev, exist := s.getAccount(addr); exist {
		new.Balance = prev.Balance
		s.repo.Put(addr, new)
	}
}

// SubBalance stub.
func (s *State) SubBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}

	account := s.getOrCreateAccount(addr)
	account.Balance = new(big.Int).Sub(account.Balance, amount)

	s.repo.Put(addr, account)
}

// AddBalance stub.
func (s *State) AddBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}

	account := s.getOrCreateAccount(addr)
	account.Balance = new(big.Int).Add(account.Balance, amount)

	s.repo.Put(addr, account)
}

// SetNonce stub.
func (s *State) SetNonce(addr common.Address, nonce uint64) {}

// SetCode stub.
func (s *State) SetCode(addr common.Address, code []byte) {
	account := s.getOrCreateAccount(addr)
	account.Code = make([]byte, len(code))
	copy(account.Code, code)

	s.repo.Put(addr, account)
}

// SetState stub.
func (s *State) SetState(addr common.Address, key common.Hash, value common.Hash) {
	s.repo.Put(StorageKey{acc.Address(addr), cry.Hash(key)}, value)
}

// Suicide stub.
func (s *State) Suicide(addr common.Address) bool {
	if account, exist := s.getAccount(addr); exist {
		acc := Account{
			Balance: account.Balance,
			Code:    account.Code}
		acc.Suicided = true

		s.repo.Put(addr, acc)

		return true
	}

	return false
}

// AddRefund stub.
func (s *State) AddRefund(gas *big.Int) {
	s.repo.Put("refund", gas)
}

// AddPreimage stub.
func (s *State) AddPreimage(hash common.Hash, preimage []byte) {
	pi := make([]byte, len(preimage))
	copy(pi, preimage)

	s.repo.Put(hash, pi)
}

// AddLog stub.
func (s *State) AddLog(log *types.Log) {
	logs, exist := s.repo.Get("logs")
	if !exist {
		s.repo.Put("logs", []*types.Log{log})
		return
	}

	newLogs := append(logs.([]*types.Log), log)
	s.repo.Put("logs", newLogs)
}

// Snapshot stub.
func (s *State) Snapshot() int {
	return s.repo.Push()
}

// RevertToSnapshot stub.
func (s *State) RevertToSnapshot(ver int) {
	s.repo.PopTo(ver)
}
