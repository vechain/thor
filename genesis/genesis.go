package genesis

import (
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var Mainnet = &mainnet{
	1519356186,
	new(big.Int).Mul(big.NewInt(86716263344), big.NewInt(1e16)), // VET 867,162,633.44
}

type mainnet struct {
	launchTime  uint64
	tokenSupply *big.Int
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
			builtin.Energy.SetTokenSupply(state, m.tokenSupply)
			return nil
		}).
		Build(stateCreator)
}
