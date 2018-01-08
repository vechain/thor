package statedb

import (
	"fmt"
	"math/big"

	"github.com/vechain/thor/vm/evm"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/stackedmap"
)

var _ evm.StateDB = (*StateDB)(nil)

// StateDB is facade for account.Manager, snapshot.Snapshot and Log.
// It implements evm.StateDB, only adapt to evm.
type StateDB struct {
	state State
	repo  *stackedmap.StackedMap
}

type suicideFlagKey common.Address
type preimageKey common.Hash
type refundKey struct{}
type logKey struct{}

// New create a statedb object.
func New(state State) *StateDB {
	getter := func(k interface{}) (interface{}, bool) {
		switch k.(type) {
		case suicideFlagKey:
			return false, true
		case refundKey:
			return &big.Int{}, true
		case preimageKey:
			return []byte(nil), true
		case logKey:
			return (*types.Log)(nil), true
		}
		panic(fmt.Sprintf("unknown type of key %+v", k))
	}

	repo := stackedmap.New(getter)
	return &StateDB{
		state,
		repo,
	}
}

// GetRefund returns total refund during VM life-cycle.
func (s *StateDB) GetRefund() *big.Int {
	v, _ := s.repo.Get(refundKey{})
	return v.(*big.Int)
}

// GetPreimages returns preimages produced by VM when evm.Config.EnablePreimageRecording turned on.
func (s *StateDB) GetPreimages() map[cry.Hash][]byte {
	preimages := make(map[cry.Hash][]byte)
	s.repo.Journal(func(k, v interface{}) bool {
		if key, ok := k.(preimageKey); ok {
			preimages[cry.Hash(key)] = v.([]byte)
		}
		return true
	})
	return preimages
}

// GetLogs return the logs collected during VM life-cycle.
func (s *StateDB) GetLogs() (logs []*types.Log) {
	s.repo.Journal(func(k, v interface{}) bool {
		if _, ok := k.(logKey); ok {
			logs = append(logs, v.(*types.Log))
		}
		return true
	})
	return
}

// ForEachStorage see state.State.ForEachStorage.
func (s *StateDB) ForEachStorage(addr common.Address, cb func(common.Hash, common.Hash) bool) {
	s.state.ForEachStorage(acc.Address(addr), func(k cry.Hash, v cry.Hash) bool {
		return cb(common.Hash(k), common.Hash(v))
	})
}

// CreateAccount stub.
func (s *StateDB) CreateAccount(addr common.Address) {}

// GetBalance stub.
func (s *StateDB) GetBalance(addr common.Address) *big.Int {
	return s.state.GetBalance(acc.Address(addr))
}

// SubBalance stub.
func (s *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	balance := s.state.GetBalance(acc.Address(addr))
	s.state.SetBalance(acc.Address(addr), new(big.Int).Sub(balance, amount))
}

// AddBalance stub.
func (s *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	balance := s.state.GetBalance(acc.Address(addr))
	s.state.SetBalance(acc.Address(addr), new(big.Int).Add(balance, amount))
}

// GetNonce stub.
func (s *StateDB) GetNonce(addr common.Address) uint64 { return 0 }

// SetNonce stub.
func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {}

// GetCodeHash stub.
func (s *StateDB) GetCodeHash(addr common.Address) common.Hash {
	return common.Hash(s.state.GetCodeHash(acc.Address(addr)))
}

// GetCode stub.
func (s *StateDB) GetCode(addr common.Address) []byte {
	return s.state.GetCode(acc.Address(addr))
}

// GetCodeSize stub.
func (s *StateDB) GetCodeSize(addr common.Address) int {
	return len(s.state.GetCode(acc.Address(addr)))
}

// SetCode stub.
func (s *StateDB) SetCode(addr common.Address, code []byte) {
	s.state.SetCode(acc.Address(addr), code)
}

// HasSuicided stub.
func (s *StateDB) HasSuicided(addr common.Address) bool {
	// only check suicide flag here
	v, _ := s.repo.Get(suicideFlagKey(addr))
	return v.(bool)
}

// Suicide stub.
// We do two things:
// 1, delete account
// 2, set suicide flag
func (s *StateDB) Suicide(addr common.Address) bool {
	if !s.state.Exists(acc.Address(addr)) {
		return false
	}
	s.state.Delete(acc.Address(addr))
	s.repo.Put(suicideFlagKey(addr), true)
	return true
}

// GetState stub.
func (s *StateDB) GetState(addr common.Address, key common.Hash) common.Hash {
	return common.Hash(s.state.GetStorage(acc.Address(addr), cry.Hash(key)))
}

// SetState stub.
func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	s.state.SetStorage(acc.Address(addr), cry.Hash(key), cry.Hash(value))
}

// Exist stub.
func (s *StateDB) Exist(addr common.Address) bool {
	return s.state.Exists(acc.Address(addr))
}

// Empty stub.
func (s *StateDB) Empty(addr common.Address) bool {
	return !s.state.Exists(acc.Address(addr))
}

// AddRefund stub.
func (s *StateDB) AddRefund(gas *big.Int) {
	v, _ := s.repo.Get(refundKey{})
	total := new(big.Int).Add(v.(*big.Int), gas)
	s.repo.Put(refundKey{}, total)
}

// AddPreimage stub.
func (s *StateDB) AddPreimage(hash common.Hash, preimage []byte) {
	s.repo.Put(preimageKey(hash), preimage)
}

// AddLog stub.
func (s *StateDB) AddLog(log *types.Log) {
	s.repo.Put(logKey{}, log)
}

// Snapshot stub.
func (s *StateDB) Snapshot() int {
	s.state.NewCheckpoint()
	rev := s.repo.Push()
	return rev
}

// RevertToSnapshot stub.
func (s *StateDB) RevertToSnapshot(rev int) {
	if rev < 0 || rev > s.repo.Depth() {
		panic(fmt.Sprintf("invalid snapshot revision %d (depth:%d)", rev, s.repo.Depth()))
	}
	revertCount := s.repo.Depth() - rev
	for i := 0; i < revertCount; i++ {
		s.state.Revert()
	}
	s.repo.PopTo(rev)
}
