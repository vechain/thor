package state

import (
	"math/big"

	"github.com/vechain/thor/cry"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/vm/account"
	"github.com/vechain/thor/vm/snapshot"
	"github.com/vechain/thor/vm/vmlog"
)

// State is facade for account.Manager, snapshot.Snapshot and Log.
// It implements evm.StateDB, only adapt to evm.
type State struct {
	accountManager  *account.Manager
	snapshotManager *snapshot.Snapshot
	vmLog           *vmlog.VMlog
}

// New create a new State object and return it's point.
func New(am *account.Manager, sm *snapshot.Snapshot, vl *vmlog.VMlog) *State {
	return &State{
		accountManager:  am,
		snapshotManager: sm,
		vmLog:           vl,
	}
}

// GetDirtiedAccounts return all the dirtied accounts.
func (s *State) GetDirtiedAccounts() []*account.Account {
	return s.accountManager.GetDirtiedAccounts()
}

// Preimages returns a list of SHA3 preimages that have been submitted.
func (s *State) Preimages() map[cry.Hash][]byte {
	return s.accountManager.Preimages()
}

// GetLogs return the log for this state.
func (s *State) GetLogs() []*types.Log {
	return s.vmLog.GetLogs()
}

// Proxy
func (s *State) CreateAccount(addr common.Address) {
	s.accountManager.CreateAccount(acc.Address(addr))
}

func (s *State) SubBalance(addr common.Address, amount *big.Int) {
	s.accountManager.SubBalance(acc.Address(addr), amount)
}

func (s *State) AddBalance(addr common.Address, amount *big.Int) {
	s.accountManager.AddBalance(acc.Address(addr), amount)
}

func (s *State) GetBalance(addr common.Address) *big.Int {
	return s.accountManager.GetBalance(acc.Address(addr))
}

func (s *State) GetNonce(addr common.Address) uint64 {
	return s.accountManager.GetNonce(acc.Address(addr))
}

func (s *State) SetNonce(addr common.Address, nonce uint64) {
	s.accountManager.SetNonce(acc.Address(addr), nonce)
}

func (s *State) GetCodeHash(addr common.Address) common.Hash {
	return common.Hash(s.accountManager.GetCodeHash(acc.Address(addr)))
}

func (s *State) GetCode(addr common.Address) []byte {
	return s.accountManager.GetCode(acc.Address(addr))
}

func (s *State) SetCode(addr common.Address, code []byte) {
	s.accountManager.SetCode(acc.Address(addr), code)
}

func (s *State) GetCodeSize(addr common.Address) int {
	return s.accountManager.GetCodeSize(acc.Address(addr))
}

func (s *State) AddRefund(gas *big.Int) {
	s.accountManager.AddRefund(gas)
}

func (s *State) GetRefund() *big.Int {
	return s.accountManager.GetRefund()
}

func (s *State) GetState(addr common.Address, hash common.Hash) common.Hash {
	return common.Hash(s.accountManager.GetState(acc.Address(addr), cry.Hash(hash)))
}

func (s *State) SetState(addr common.Address, key common.Hash, value common.Hash) {
	s.accountManager.SetState(acc.Address(addr), cry.Hash(key), cry.Hash(value))
}

func (s *State) Suicide(addr common.Address) bool {
	s.accountManager.Suicide(acc.Address(addr))
	return true
}

func (s *State) HasSuicided(addr common.Address) bool {
	return s.accountManager.HasSuicided(acc.Address(addr))
}

func (s *State) Exist(addr common.Address) bool {
	return s.accountManager.Exist(acc.Address(addr))
}

func (s *State) Empty(addr common.Address) bool {
	return s.accountManager.Empty(acc.Address(addr))
}

func (s *State) ForEachStorage(addr common.Address, cb func(common.Hash, common.Hash) bool) {
	s.accountManager.ForEachStorage(acc.Address(addr), func(key cry.Hash, value cry.Hash) bool {
		return cb(common.Hash(key), common.Hash(value))
	})
}

func (s *State) AddPreimage(hash common.Hash, preimage []byte) {
	s.accountManager.AddPreimage(cry.Hash(hash), preimage)
}

func (s *State) Snapshot() int {
	return s.snapshotManager.Snapshot(s.accountManager)
}

func (s *State) RevertToSnapshot(ver int) {
	s.accountManager = s.snapshotManager.RevertToSnapshot(ver).(*account.Manager)
}

func (s *State) AddLog(log *types.Log) {
	s.vmLog.AddLog(log)
}
