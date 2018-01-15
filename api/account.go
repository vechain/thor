package api

import (
	"math/big"

	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

//AccountInterface manage accounts
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
func (ai *AccountInterface) GetBalance(address thor.Address) *big.Int {
	return ai.state.GetBalance(address)
}

//GetCode return code from account
func (ai *AccountInterface) GetCode(address thor.Address) []byte {
	return ai.state.GetCode(address)
}

//GetStorage return storage value from key
func (ai *AccountInterface) GetStorage(address thor.Address, key thor.Hash) thor.Hash {
	return ai.state.GetStorage(address, key)
}
