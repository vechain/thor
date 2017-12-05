package account

// import (
// 	"math/big"

// 	"github.com/ethereum/go-ethereum/common"
// 	"github.com/ethereum/go-ethereum/core/types"
// 	"github.com/vechain/vecore/acc"
// 	"github.com/vechain/vecore/vm/account"
// )

// // Preimager is Preimage for sha3.
// type Preimager interface {
// 	AddPreimage(common.Address, []byte)
// }

// // Journaler is log for operation.
// type Journaler interface {
// 	AddLog(*types.Log)
// }

// // Snapshoter is version for evm.
// type Snapshoter interface {
// 	AddSnapshot(interface{})
// 	Fullback() interface{}
// }

// // AccountManager is account's delegate.
// type AccountManager interface {
// 	DeepCopy() interface{}
// 	GetDirtiedAccounts() []*account.Account

// 	// AddRefund(*big.Int)
// 	// GetRefund() *big.Int

// 	// // Delegated to the account
// 	// CreateAccount(common.Address)

// 	// SubBalance(common.Address, *big.Int)
// 	AddBalance(acc.Address, *big.Int)
// 	// 	GetBalance(common.Address) *big.Int

// 	// 	GetNonce(common.Address) uint64
// 	// 	SetNonce(common.Address, uint64)

// 	// 	GetCodeHash(common.Address) common.Hash
// 	// 	GetCode(common.Address) []byte
// 	// 	SetCode(common.Address, []byte)
// 	// 	GetCodeSize(common.Address) int

// 	// 	GetState(common.Address, common.Hash) common.Hash
// 	// 	SetState(common.Address, common.Hash, common.Hash)

// 	// 	Suicide(common.Address) bool // 删除账户
// 	// 	HasSuicided(common.Address) bool

// 	// 	// Exist reports whether the given account exists in state.
// 	// 	// Notably this should also return true for suicided accounts.
// 	// 	Exist(common.Address) bool
// 	// 	// Empty returns whether the given account is empty. Empty
// 	// 	// is defined according to EIP161 (balance = nonce = code = 0).
// 	// 	Empty(common.Address) bool

// 	// 	ForEachStorage(common.Address, func(common.Hash, common.Hash) bool)
// }
