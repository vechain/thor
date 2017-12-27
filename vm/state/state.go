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
func New(stater StateReader) *State {
	getter := func(hash interface{}) (interface{}, bool) {
		switch v := hash.(type) {
		case common.Address:
			if exist := stater.Exist(acc.Address(v)); exist == false {
				return nil, false
			}
			return newAccount(stater.GetBalance(acc.Address(v)), stater.GetCode(acc.Address(v))), true
		case StorageKey:
			return common.Hash(stater.GetStorage(v.Addr, v.Key)), true
		}
		return nil, false
	}

	return &State{
		repo: stackedmap.New(getter),
	}
}

func (s *State) getAccount(addr common.Address) *Account {
	account, exist := s.repo.Get(addr)
	if !exist {
		return nil
	}
	return account.(*Account)
}

// GetAccountAndStorage return all the dirty.
func (s *State) GetAccountAndStorage() (map[acc.Address]*Account, map[StorageKey]cry.Hash) {
	accounts := make(map[acc.Address]*Account)
	storage := make(map[StorageKey]cry.Hash)
	for key, value := range s.repo.Puts() {
		switch v := key.(type) {
		case common.Address:
			accounts[acc.Address(v)] = value.(*Account)
		case StorageKey:
			storage[v] = cry.Hash(value.(common.Hash))
		}
	}
	return accounts, storage
}

// GetRefund stub.
func (s *State) GetRefund() *big.Int {
	refund, exist := s.repo.Get("refund")
	if !exist {
		return big.NewInt(0)
	}
	return refund.(*big.Int)
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
	logs, exist := s.repo.Get("logs")
	if !exist {
		return nil
	}
	return logs.([]*types.Log)
}

// ForEachStorage stub.
func (s *State) ForEachStorage(addr common.Address, cb func(common.Hash, common.Hash) bool) {
	panic("ForEachStorage Unrealized!")
}

// GetBalance stub.
func (s *State) GetBalance(addr common.Address) *big.Int {
	account := s.getAccount(addr)
	if account == nil {
		return big.NewInt(0)
	}
	return account.Balance()
}

// GetNonce stub.
func (s *State) GetNonce(addr common.Address) uint64 {
	return 0
}

// GetCodeHash stub.
func (s *State) GetCodeHash(addr common.Address) common.Hash {
	account := s.getAccount(addr)
	if account == nil {
		return common.Hash{}
	}
	return crypto.Keccak256Hash(account.Code())
}

// GetCode stub.
func (s *State) GetCode(addr common.Address) []byte {
	account := s.getAccount(addr)
	if account == nil {
		return nil
	}
	return account.Code()
}

// GetCodeSize stub.
func (s *State) GetCodeSize(addr common.Address) int {
	account := s.getAccount(addr)
	if account == nil {
		return 0
	}
	return len(account.Code())
}

// HasSuicided stub.
func (s *State) HasSuicided(addr common.Address) bool {
	account := s.getAccount(addr)
	if account != nil {
		return account.Suicided()
	}
	return false
}

// Empty stub.
func (s *State) Empty(addr common.Address) bool {
	account := s.getAccount(addr)
	if account == nil {
		return true
	}
	return account.Balance().Sign() == 0 && account.Code() == nil
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

func (s *State) withAccount(addr common.Address) *Account {
	account := s.getAccount(addr)
	if account != nil {
		return newAccount(account.Balance(), account.Code())
	}
	return newAccount(big.NewInt(0), nil)
}

// CreateAccount stub.
func (s *State) CreateAccount(addr common.Address) {
	new := newAccount(big.NewInt(0), nil)
	prev := s.getAccount(addr)
	if prev != nil {
		new.setBalance(prev.Balance())
		s.repo.Put(addr, new)
	}
}

// SubBalance stub.
func (s *State) SubBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}

	account := s.withAccount(addr)
	newBalance := new(big.Int).Sub(account.Balance(), amount)
	account.setBalance(newBalance)

	s.repo.Put(addr, account)
}

// AddBalance stub.
func (s *State) AddBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}

	account := s.withAccount(addr)
	newBalance := new(big.Int).Add(account.Balance(), amount)
	account.setBalance(newBalance)

	s.repo.Put(addr, account)
}

// SetNonce stub.
func (s *State) SetNonce(addr common.Address, nonce uint64) {}

// SetCode stub.
func (s *State) SetCode(addr common.Address, code []byte) {
	account := s.withAccount(addr)
	ce := make([]byte, len(code))
	copy(ce, code)
	account.setCode(ce)

	s.repo.Put(addr, account)
}

// SetState stub.
func (s *State) SetState(addr common.Address, key common.Hash, value common.Hash) {
	s.repo.Put(StorageKey{acc.Address(addr), cry.Hash(key)}, value)
}

// Suicide stub.
func (s *State) Suicide(addr common.Address) bool {
	account := s.getAccount(addr)
	if account == nil {
		return false
	}

	acc := newAccount(account.Balance(), account.Code())
	acc.setSuicided()

	s.repo.Put(addr, acc)

	return true
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
