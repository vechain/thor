package api

import (
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/state"
)

//AccountInterface manage account
type AccountInterface struct {
	state *state.State
}

//NewAccountInterface create new AccountManager
func NewAccountInterface(state *state.State) *AccountInterface {
	return &AccountInterface{
		state: state,
	}
}

//GetAccount get account by address
func (ai *AccountInterface) GetAccount(address acc.Address) *acc.Account {
	return ai.state.GetAccount(address)
}
