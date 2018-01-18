package genesis

import (
	"math/big"

	"github.com/vechain/thor/block"
	cs "github.com/vechain/thor/contracts"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

const (
	// Timestamp timestamp of genesis block.
	Timestamp uint64 = 1234567890
)

// Build build the genesis block.
func Build(state *state.State) (*block.Block, error) {
	return new(Builder).
		Timestamp(Timestamp).
		GasLimit(thor.InitialGasLimit).
		Alloc(
			cs.Authority.Address,
			&big.Int{},
			cs.Authority.RuntimeBytecodes(),
		).
		Alloc(
			cs.Energy.Address,
			&big.Int{},
			cs.Energy.RuntimeBytecodes(),
		).
		Alloc(
			cs.Params.Address,
			&big.Int{},
			cs.Params.RuntimeBytecodes(),
		).
		Call(
			cs.Authority.Address,
			cs.Authority.PackInitialize(thor.Address{} /*TODO*/),
		).
		Call(
			cs.Params.Address,
			cs.Params.PackPreset(cs.ParamRewardPercentage, big.NewInt(30)),
		).
		Build(state)
}
