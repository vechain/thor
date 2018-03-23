package api

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

//AccountInterface manage accounts
type AccountInterface struct {
	chain        *chain.Chain
	stateCreator *state.Creator
}

//NewAccountInterface create new AccountManager
func NewAccountInterface(chain *chain.Chain, stateCreator *state.Creator) *AccountInterface {
	return &AccountInterface{
		chain,
		stateCreator,
	}
}

//GetStorage return storage value from key
func (ai *AccountInterface) GetStorage(address thor.Address, key thor.Hash) thor.Hash {
	bestBlk, err := ai.chain.GetBestBlock()
	if err != nil {
		return thor.Hash{}
	}
	state, err := ai.stateCreator.NewState(bestBlk.Header().StateRoot())
	if err != nil {
		return thor.Hash{}
	}
	return state.GetStorage(address, key)
}

//GetBalance returns balance by address
func (ai *AccountInterface) GetBalance(address thor.Address) *math.HexOrDecimal256 {
	bestBlk, err := ai.chain.GetBestBlock()
	if err != nil {
		return nil
	}
	state, err := ai.stateCreator.NewState(bestBlk.Header().StateRoot())
	if err != nil {
		return nil
	}
	b := state.GetBalance(address)
	balance := math.HexOrDecimal256(*b)
	return &balance
}

//GetCode returns code by address
func (ai *AccountInterface) GetCode(address thor.Address) []byte {
	bestBlk, err := ai.chain.GetBestBlock()
	if err != nil {
		return nil
	}
	state, err := ai.stateCreator.NewState(bestBlk.Header().StateRoot())
	if err != nil {
		return nil
	}
	return state.GetCode(address)
}
