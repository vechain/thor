package vcc

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/state"
)

//AccountManager manage account
type AccountManager struct {
	State *state.State
}

//GetAccount get account by address
func (accountManager *AccountManager) GetAccount(address acc.Address) *acc.Account {
	return accountManager.State.GetAccount(address)
}
