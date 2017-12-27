package state

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// StorageKey is composite keys for storage.
type StorageKey struct {
	Addr acc.Address
	Key  cry.Hash
}

// Account manage acc.Account and Storage.
type Account struct {
	balance  *big.Int
	code     []byte
	suicided bool
}

func newAccount(balance *big.Int, code []byte) *Account {
	return &Account{
		balance:  balance,
		code:     code,
		suicided: false,
	}
}

// Balance is balance's getter.
func (acc *Account) Balance() *big.Int {
	return acc.balance
}

// Code is code's getter.
func (acc *Account) Code() []byte {
	return acc.code
}

// Suicided is suicided's getter.
func (acc *Account) Suicided() bool {
	return acc.suicided
}

func (acc *Account) setSuicided() {
	acc.suicided = true
}

func (acc *Account) setCode(code []byte) {
	acc.code = code
}

func (acc *Account) setBalance(balance *big.Int) {
	acc.balance = balance
}
