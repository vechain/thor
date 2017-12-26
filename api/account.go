package api

import (
	"math/big"

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

//GetBalance return balance from account
func (ai *AccountInterface) GetBalance(address acc.Address) *big.Int {
	return ai.state.GetBalance(address)
}

//GetCode return code from account
func (ai *AccountInterface) GetCode(address acc.Address) []byte {
	return ai.state.GetCode(address)
}
