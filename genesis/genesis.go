package genesis

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var Mainnet = &mainnet{1519356186}

type mainnet struct {
	launchTime uint64
}

func (m *mainnet) Build(stateCreator *state.Creator) (*block.Block, error) {
	return new(Builder).
		ChainTag(1).
		Timestamp(m.launchTime).
		GasLimit(thor.InitialGasLimit).
		State(func(state *state.State) error {
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
			state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())

			builtin.Params.Set(state, thor.KeyRewardRatio, thor.InitialRewardRatio)
			builtin.Params.Set(state, thor.KeyBaseGasPrice, thor.InitialBaseGasPrice)
			builtin.Energy.AdjustGrowthRate(state, m.launchTime, thor.InitialEnergyGrowthRate)
			return nil
		}).
		Build(stateCreator)
}
