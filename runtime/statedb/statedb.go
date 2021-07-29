// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package statedb

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/stackedmap"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var codeSizeCache, _ = lru.New(32 * 1024)

// StateDB implements evm.StateDB, only adapt to evm.
type StateDB struct {
	state *state.State
	repo  *stackedmap.StackedMap
}

type (
	suicideFlagKey common.Address
	refundKey      struct{}
	preimageKey    common.Hash
	eventKey       struct{}
	transferKey    struct{}
	stateRevKey    struct{}
)

// New create a statedb object.
func New(state *state.State) *StateDB {
	getter := func(k interface{}) (interface{}, bool, error) {
		switch k.(type) {
		case suicideFlagKey:
			return false, true, nil
		case refundKey:
			return uint64(0), true, nil
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
func (s *StateDB) GetRefund() uint64 {
	v, _, _ := s.repo.Get(refundKey{})
	return v.(uint64)
}

// GetLogs returns collected event and transfer logs.
func (s *StateDB) GetLogs() (tx.Events, tx.Transfers) {
	var (
		events    tx.Events
		transfers tx.Transfers
	)
	s.repo.Journal(func(k, v interface{}) bool {
		switch k.(type) {
		case eventKey:
			events = append(events, ethlogToEvent(v.(*types.Log)))
		case transferKey:
			transfers = append(transfers, v.(*tx.Transfer))
		}
		return true
	})
	return events, transfers
}

// ForEachStorage see state.State.ForEachStorage.
// func (s *StateDB) ForEachStorage(addr common.Address, cb func(common.Hash, common.Hash) bool) {
// 	s.state.ForEachStorage(thor.Address(addr), func(k thor.Bytes32, v []byte) bool {
// 		// TODO should rlp decode v
// 		return cb(common.Hash(k), common.BytesToHash(v))
// 	})
// }

// CreateAccount stub.
func (s *StateDB) CreateAccount(addr common.Address) {}

// GetBalance stub.
func (s *StateDB) GetBalance(addr common.Address) *big.Int {
	bal, err := s.state.GetBalance(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	return bal
}

// SubBalance stub.
func (s *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	balance, err := s.state.GetBalance(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	if err := s.state.SetBalance(thor.Address(addr), new(big.Int).Sub(balance, amount)); err != nil {
		panic(err)
	}
}

// AddBalance stub.
func (s *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	balance, err := s.state.GetBalance(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	if err := s.state.SetBalance(thor.Address(addr), new(big.Int).Add(balance, amount)); err != nil {
		panic(err)
	}
}

// GetNonce stub.
func (s *StateDB) GetNonce(addr common.Address) uint64 { return 0 }

// SetNonce stub.
func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {}

// GetCodeHash stub.
func (s *StateDB) GetCodeHash(addr common.Address) common.Hash {
	hash, err := s.state.GetCodeHash(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	return common.Hash(hash)
}

// GetCode stub.
func (s *StateDB) GetCode(addr common.Address) []byte {
	code, err := s.state.GetCode(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	return code
}

// GetCodeSize stub.
func (s *StateDB) GetCodeSize(addr common.Address) int {
	hash, err := s.state.GetCodeHash(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	if hash.IsZero() {
		return 0
	}
	if v, ok := codeSizeCache.Get(hash); ok {
		return v.(int)
	}
	code, err := s.state.GetCode(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	size := len(code)
	codeSizeCache.Add(hash, size)
	return size
}

// SetCode stub.
func (s *StateDB) SetCode(addr common.Address, code []byte) {
	if err := s.state.SetCode(thor.Address(addr), code); err != nil {
		panic(err)
	}
}

// HasSuicided stub.
func (s *StateDB) HasSuicided(addr common.Address) bool {
	// only check suicide flag here
	v, _, _ := s.repo.Get(suicideFlagKey(addr))
	return v.(bool)
}

// Suicide stub.
// We do two things:
// 1, delete account
// 2, set suicide flag
func (s *StateDB) Suicide(addr common.Address) bool {
	exist, err := s.state.Exists(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	if !exist {
		return false
	}
	s.state.Delete(thor.Address(addr))
	s.repo.Put(suicideFlagKey(addr), true)
	return true
}

// GetState stub.
func (s *StateDB) GetState(addr common.Address, key common.Hash) common.Hash {
	val, err := s.state.GetStorage(thor.Address(addr), thor.Bytes32(key))
	if err != nil {
		panic(err)
	}
	return common.Hash(val)
}

// SetState stub.
func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	s.state.SetStorage(thor.Address(addr), thor.Bytes32(key), thor.Bytes32(value))
}

// Exist stub.
func (s *StateDB) Exist(addr common.Address) bool {
	b, err := s.state.Exists(thor.Address(addr))
	if err != nil {
		panic(err)
	}
	return b
}

// Empty stub.
func (s *StateDB) Empty(addr common.Address) bool {
	return !s.Exist(addr)
}

// AddRefund stub.
func (s *StateDB) AddRefund(gas uint64) {
	v, _, _ := s.repo.Get(refundKey{})
	total := v.(uint64) + gas
	s.repo.Put(refundKey{}, total)
}

// AddPreimage stub.
func (s *StateDB) AddPreimage(hash common.Hash, preimage []byte) {
	s.repo.Put(preimageKey(hash), preimage)
}

// AddLog stub.
func (s *StateDB) AddLog(vmlog *types.Log) {
	s.repo.Put(eventKey{}, vmlog)
}

func (s *StateDB) AddTransfer(transfer *tx.Transfer) {
	s.repo.Put(transferKey{}, transfer)
}

// Snapshot stub.
func (s *StateDB) Snapshot() int {
	srev := s.state.NewCheckpoint()
	s.repo.Put(stateRevKey{}, srev)
	return s.repo.Push()
}

// RevertToSnapshot stub.
func (s *StateDB) RevertToSnapshot(rev int) {
	s.repo.PopTo(rev)
	if srev, ok, _ := s.repo.Get(stateRevKey{}); ok {
		s.state.RevertTo(srev.(int))
	} else {
		panic("state checkpoint missing")
	}
}

func ethlogToEvent(ethlog *types.Log) *tx.Event {
	var topics []thor.Bytes32
	if len(ethlog.Topics) > 0 {
		topics = make([]thor.Bytes32, 0, len(ethlog.Topics))
		for _, t := range ethlog.Topics {
			topics = append(topics, thor.Bytes32(t))
		}
	}
	return &tx.Event{
		Address: thor.Address(ethlog.Address),
		Topics:  topics,
		Data:    ethlog.Data,
	}
}
