package genesis

import (
	"math/big"

	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm/evm"
)

// NewTestnet create genesis for testnet.
func NewTestnet() (*Genesis, error) {

	launchTime := uint64(1526227200) // Mon May 14 2018 00:00:00 GMT+0800 (CST)
	tokenSupply := new(big.Int)

	builtin.Executor.Address, _ = thor.ParseAddress("0xB5A34b62b63A6f1EE99DFD30b133B657859f8d79")
	acccount0, _ := thor.ParseAddress("0xe59D475Abe695c7f67a8a2321f33A856B0B4c71d")

	master0, _ := thor.ParseAddress("0x25AE0ef84dA4a76D5a1DFE80D3789C2c46FeE30a")
	endorser0, _ := thor.ParseAddress("0xb4094c25f86d628fdD571Afc4077f0d0196afB48")

	builder := new(Builder).
		Timestamp(launchTime).
		GasLimit(thor.InitialGasLimit).
		State(func(state *state.State) error {
			// alloc precompiled contracts
			for addr := range evm.PrecompiledContractsByzantium {
				state.SetCode(thor.Address(addr), emptyRuntimeBytecode)
			}
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
			state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
			state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes())
			state.SetCode(builtin.Executor.Address, builtin.Executor.RuntimeBytecodes())
			state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes())

			// 1 million
			amount := new(big.Int).Mul(big.NewInt(1e18), big.NewInt(1000*1000))
			state.SetBalance(builtin.Executor.Address, amount)
			tokenSupply.Add(tokenSupply, amount)

			// 1 billion
			amount = new(big.Int).Mul(big.NewInt(1e18), big.NewInt(1000*1000*1000))
			state.SetBalance(acccount0, amount)
			tokenSupply.Add(tokenSupply, amount)

			amount = new(big.Int).Mul(big.NewInt(1e18), big.NewInt(250*1000))
			state.SetBalance(endorser0, amount)
			tokenSupply.Add(tokenSupply, amount)

			builtin.Energy.Native(state).SetInitialSupply(tokenSupply, &big.Int{})
			return nil
		}).
		Call(
			tx.NewClause(&builtin.Params.Address).
				WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyRewardRatio, thor.InitialRewardRatio)),
			builtin.Executor.Address).
		Call(
			tx.NewClause(&builtin.Params.Address).
				WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyBaseGasPrice, thor.InitialBaseGasPrice)),
			builtin.Executor.Address).
		Call(
			tx.NewClause(&builtin.Params.Address).
				WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyProposerEndorsement, thor.InitialProposerEndorsement)),
			builtin.Executor.Address).
		Call(tx.NewClause(&builtin.Authority.Address).
			WithData(mustEncodeInput(builtin.Authority.ABI, "add", master0, endorser0, thor.BytesToBytes32([]byte("master0")))),
			builtin.Executor.Address)

	id, err := builder.ComputeID()
	if err != nil {
		return nil, err
	}
	return &Genesis{builder, id, "testnet"}, nil
}
