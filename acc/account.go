package acc

import (
	"math/big"

	"github.com/vechain/thor/cry"
)

//Account Thor account
type Account struct {
	Balance     *big.Int
	CodeHash    cry.Hash
	StorageRoot cry.Hash // merkle root of the storage trie
}

// Account Set Balance
func (account *Account) setBalance(balance *big.Int) {
	account.Balance = balance
}

//SubBalance Account
func (account *Account) SubBalance(balance *big.Int) {
	if balance.Sign() == 0 {
		return
	}
	account.setBalance(new(big.Int).Sub(account.Balance, balance))
}

// AddBalance Account
func (account *Account) AddBalance(balance *big.Int) {
	if balance.Sign() == 0 {
		return
	}
	account.setBalance(new(big.Int).Add(account.Balance, balance))
}
