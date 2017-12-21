package api

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/state"
)

//AccountManager manage account
type AccountManager struct {
	state *state.State
}

//New create new AccountManager
func New(state *state.State) *AccountManager {
	return &AccountManager{
		state: state,
	}
}

//GetAccount get account by address
func (am *AccountManager) GetAccount(address acc.Address) *acc.Account {
	return am.state.GetAccount(address)
}
