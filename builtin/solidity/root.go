package solidity

import (
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type Root struct {
	address thor.Address
	state *state.State
	charger *gascharger.Charger
}

func NewRoot(address thor.Address, state *state.State, charger *gascharger.Charger) *Root {
	return &Root{
		address: address,
		state:   state,
		charger: charger,
	}
}

