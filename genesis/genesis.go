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
	genesis, err := new(Builder).
		Timestamp(1517304350).
		GasLimit(thor.InitialGasLimit).
		/// deploy
		Alloc(cs.Authority.Address, &big.Int{}, cs.Authority.RuntimeBytecodes()).
		Alloc(cs.Energy.Address, &big.Int{}, cs.Energy.RuntimeBytecodes()).
		Alloc(cs.Params.Address, &big.Int{}, cs.Params.RuntimeBytecodes()).
		/// initialize
		Call(cs.Authority.PackInitialize(cs.Voting.Address)).
		Call(cs.Energy.PackInitialize(cs.Voting.Address)).
		Call(cs.Params.PackInitialize(cs.Voting.Address)).
		/// preset
		Call(cs.Params.PackPreset(cs.ParamRewardRatio, big.NewInt(3e17))).
		Call(cs.Params.PackPreset(cs.ParamBaseGasPrice, big.NewInt(1000))).
		Build(stateCreator)
	if err != nil {
		return nil, err
	}
	// set chain tag to 1
	return genesis.WithSignature([]byte{1}), nil
}
