package api

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"math/big"
)

type bestBlockGetter interface {
	GetBestBlock() (*block.Block, error)
}

//AccountInterface manage accounts
type AccountInterface struct {
	bestBlkGetter bestBlockGetter
	stateCreator  func(thor.Hash) *state.State
}

//NewAccountInterface create new AccountManager
func NewAccountInterface(bestBlkGetter bestBlockGetter, stateCreator func(thor.Hash) *state.State) *AccountInterface {
	return &AccountInterface{
		bestBlkGetter,
		stateCreator,
	}
}

//GetBalance return balance from account
func (ai *AccountInterface) GetBalance(address thor.Address) *big.Int {
	bestBlk, err := ai.bestBlkGetter.GetBestBlock()
	if err != nil {
		return new(big.Int)
	}
	return ai.stateCreator(bestBlk.Header().StateRoot()).GetBalance(address)
}

//GetCode return code from account
func (ai *AccountInterface) GetCode(address thor.Address) []byte {
	bestBlk, err := ai.bestBlkGetter.GetBestBlock()
	if err != nil {
		return nil
	}
	return ai.stateCreator(bestBlk.Header().StateRoot()).GetCode(address)
}

//GetStorage return storage value from key
func (ai *AccountInterface) GetStorage(address thor.Address, key thor.Hash) thor.Hash {
	bestBlk, err := ai.bestBlkGetter.GetBestBlock()
	if err != nil {
		return thor.Hash{}
	}
	return ai.stateCreator(bestBlk.Header().StateRoot()).GetStorage(address, key)
}
