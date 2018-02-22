package genesis

import (
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var Mainnet = &mainnet{}

type mainnet struct{}

func (m *mainnet) Build(stateCreator *state.Creator) (*block.Block, error) {
	return new(Builder).
		Timestamp(1517304350).
		GasLimit(thor.InitialGasLimit).
		State(func(state *state.State) error {
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
			state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())

			builtin.Params.Set(state, thor.KeyRewardRatio, big.NewInt(3e17))
			builtin.Params.Set(state, thor.KeyBaseGasPrice, big.NewInt(1000))
			return nil
		}).
		Build(stateCreator)
}
