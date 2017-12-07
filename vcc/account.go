package vcc

import (
	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/state"
)

//AccountManager manage account
type AccountManager struct {
	State *state.State
}

//GetAccount get account by address
func (accountManager *AccountManager) GetAccount(address acc.Address) *acc.Account {
	return accountManager.State.GetAccount(address)
}
