package genesis

import (
	"math/big"

	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm/evm"
)

var Mainnet = &mainnet{
	1519356186,
	new(big.Int).Mul(big.NewInt(86716263344), big.NewInt(1e16)), // VET 867,162,633.44
}

type mainnet struct {
	launchTime  uint64
	tokenSupply *big.Int
}

func (m *mainnet) Build(stateCreator *state.Creator) (*block.Block, tx.Logs, error) {
	return new(Builder).
		Timestamp(m.launchTime).
		GasLimit(thor.InitialGasLimit).
		State(func(state *state.State) error {
			// alloc precompiled contracts
			for addr := range evm.PrecompiledContractsByzantium {
				state.SetBalance(thor.Address(addr), big.NewInt(1))
			}
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
			state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())

			builtin.Energy.WithState(state).InitializeTokenSupply(m.launchTime, m.tokenSupply)
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
		Build(stateCreator)
}

func mustEncodeInput(abi *abi.ABI, name string, args ...interface{}) []byte {
	m := abi.MethodByName(name)
	if m == nil {
		panic("no method '" + name + "'")
	}
	data, err := m.EncodeInput(args...)
	if err != nil {
		panic(err)
	}
	return data
}
