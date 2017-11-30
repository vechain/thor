package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// AccountManager is.
type AccountManager interface {
	AddRefund(*big.Int)
	GetRefund() *big.Int
	AddPreimage(common.Hash, []byte)

	// Delegated to the account
	CreateAccount(common.Address)

	SubBalance(common.Address, *big.Int)
	AddBalance(common.Address, *big.Int)
	GetBalance(common.Address) *big.Int

	GetNonce(common.Address) uint64
	SetNonce(common.Address, uint64)

	GetCodeHash(common.Address) common.Hash
	GetCode(common.Address) []byte
	SetCode(common.Address, []byte)
	GetCodeSize(common.Address) int

	GetState(common.Address, common.Hash) common.Hash
	SetState(common.Address, common.Hash, common.Hash)

	Suicide(common.Address) bool // 删除账户
	HasSuicided(common.Address) bool

	// Exist reports whether the given account exists in state.
	// Notably this should also return true for suicided accounts.
	Exist(common.Address) bool
	// Empty returns whether the given account is empty. Empty
	// is defined according to EIP161 (balance = nonce = code = 0).
	Empty(common.Address) bool

	ForEachStorage(common.Address, func(common.Hash, common.Hash) bool)
}

// Journaler is log for operation.
type Journaler interface {
	AddLog(*types.Log)
}

// Snapshoter is version control.
type Snapshoter interface {
	RevertToSnapshot(int)
	Snapshot() int
}
