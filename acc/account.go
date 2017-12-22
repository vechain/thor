package acc

import (
	"math/big"

	"github.com/vechain/thor/cry"
)

// EmptyCodeHash hash of empty code
var EmptyCodeHash = cry.HashSum(nil)

//Account Thor account
type Account struct {
	Balance     *big.Int
	CodeHash    cry.Hash
	StorageRoot cry.Hash // merkle root of the storage trie
}

//SubBalance Account
func (a *Account) SubBalance(balance *big.Int) {
	if balance.Sign() == 0 {
		return
	}
	a.Balance = new(big.Int).Sub(a.Balance, balance)
}

// AddBalance Account
func (a *Account) AddBalance(balance *big.Int) {
	if balance.Sign() == 0 {
		return
	}
	a.Balance = new(big.Int).Add(a.Balance, balance)
}

// IsEmpty returns if an account is empty.
// Similar to EIP158, but here we don't have nonce.
func (a *Account) IsEmpty() bool {
	return a.Balance.Sign() == 0 && a.CodeHash == EmptyCodeHash
}
