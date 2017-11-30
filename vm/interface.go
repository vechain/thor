package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/vecore/acc"
)

// IPreimager is Preimage for sha3.
type IPreimager interface {
	AddPreimage(acc.Address, []byte)
}

// IJournaler is log for operation.
type IJournaler interface {
	AddLog(*types.Log)
}

// IAccountManager is account's delegate.
type IAccountManager interface {
	AddRefund(*big.Int)
	GetRefund() *big.Int

	// Delegated to the account
	CreateAccount(acc.Address)

	SubBalance(acc.Address, *big.Int)
	AddBalance(acc.Address, *big.Int)
	GetBalance(acc.Address) *big.Int

	GetNonce(acc.Address) uint64
	SetNonce(acc.Address, uint64)

	GetCodeHash(acc.Address) common.Hash
	GetCode(acc.Address) []byte
	SetCode(acc.Address, []byte)
	GetCodeSize(acc.Address) int

	GetState(acc.Address, common.Hash) common.Hash
	SetState(acc.Address, common.Hash, common.Hash)

	Suicide(acc.Address) bool // 删除账户
	HasSuicided(acc.Address) bool

	// Exist reports whether the given account exists in state.
	// Notably this should also return true for suicided accounts.
	Exist(acc.Address) bool
	// Empty returns whether the given account is empty. Empty
	// is defined according to EIP161 (balance = nonce = code = 0).
	Empty(acc.Address) bool

	ForEachStorage(acc.Address, func(common.Hash, common.Hash) bool)
}

// ISnapshoter is version control.
type ISnapshoter interface {
	RevertToSnapshot(int)
	Snapshot() int
}
