package api

import (
	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type bestBlockGetter interface {
	GetBestBlock() (*block.Block, error)
}

//AccountInterface manage accounts
type AccountInterface struct {
	bestBlkGetter bestBlockGetter
	stateCreator  *state.Creator
}

//NewAccountInterface create new AccountManager
func NewAccountInterface(bestBlkGetter bestBlockGetter, stateCreator *state.Creator) *AccountInterface {
	return &AccountInterface{
		bestBlkGetter,
		stateCreator,
	}
}

//GetStorage return storage value from key
func (ai *AccountInterface) GetStorage(address thor.Address, key thor.Hash) thor.Hash {
	bestBlk, err := ai.bestBlkGetter.GetBestBlock()
	if err != nil {
		return thor.Hash{}
	}
	state, err := ai.stateCreator.NewState(bestBlk.Header().StateRoot())
	if err != nil {
		return thor.Hash{}
	}
	return state.GetStorage(address, key)
}

//GetAccount returns account by address
func (ai *AccountInterface) GetAccount(address thor.Address) *types.Account {
	bestBlk, err := ai.bestBlkGetter.GetBestBlock()
	if err != nil {
		return nil
	}
	state, err := ai.stateCreator.NewState(bestBlk.Header().StateRoot())
	if err != nil {
		return nil
	}
	balance := state.GetBalance(address)
	code := state.GetCode(address)
	return &types.Account{
		Balance: balance,
		Code:    code,
	}
}
