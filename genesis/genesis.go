package genesis

import (
	"math/big"

	"github.com/vechain/thor/block"
	cs "github.com/vechain/thor/contracts"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var Mainnet = &mainnet{}

type mainnet struct {
}

func (m *mainnet) Build(stateCreator *state.Creator) (*block.Block, error) {
	return new(Builder).
		Timestamp(1517304350).
		GasLimit(thor.InitialGasLimit).
		State(func(state *state.State) error {
			state.SetCode(cs.Authority.Address, cs.Authority.RuntimeBytecodes())
			state.SetCode(cs.Energy.Address, cs.Energy.RuntimeBytecodes())
			state.SetCode(cs.Params.Address, cs.Params.RuntimeBytecodes())

			cs.Params.Set(state, cs.ParamRewardRatio, big.NewInt(3e17))
			cs.Params.Set(state, cs.ParamBaseGasPrice, big.NewInt(1000))
			return nil
		}).
		Build(stateCreator)
}
