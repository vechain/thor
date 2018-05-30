// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/pkg/errors"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm/evm"
)

// TransactionEnv transaction environment.
type TransactionEnv struct {
	ID         thor.Bytes32
	Origin     thor.Address
	GasPrice   *big.Int
	ProvedWork *big.Int
}

type environment struct {
	abi      *abi.Method
	seeker   *chain.Seeker
	state    *state.State
	evm      *evm.EVM
	contract *evm.Contract
	txEnv    *TransactionEnv
}

func newEnvironment(
	abi *abi.Method,
	seeker *chain.Seeker,
	state *state.State,
	evm *evm.EVM,
	contract *evm.Contract,
	txEnv *TransactionEnv,
) *environment {
	return &environment{
		abi:      abi,
		seeker:   seeker,
		state:    state,
		evm:      evm,
		contract: contract,
		txEnv:    txEnv,
	}
}

func (env *environment) UseGas(gas uint64) {
	if !env.contract.UseGas(gas) {
		panic(&vmError{evm.ErrOutOfGas})
	}
}

func (env *environment) ParseArgs(val interface{}) {
	if err := env.abi.DecodeInput(env.contract.Input, val); err != nil {
		// as vm error
		panic(&vmError{errors.WithMessage(err, "decode native input")})
	}
}

func (env *environment) Require(cond bool) {
	if !cond {
		panic(&vmError{evm.ErrExecutionReverted()})
	}
}

func (env *environment) Log(abi *abi.Event, address thor.Address, topics []thor.Bytes32, args ...interface{}) {
	data, err := abi.Encode(args...)
	if err != nil {
		panic(errors.WithMessage(err, "encode native event"))
	}
	env.UseGas(ethparams.LogGas + ethparams.LogTopicGas*uint64(len(topics)) + ethparams.LogDataGas*uint64(len(data)))

	ethTopics := make([]common.Hash, 0, len(topics)+1)
	ethTopics = append(ethTopics, common.Hash(abi.ID()))
	for _, t := range topics {
		ethTopics = append(ethTopics, common.Hash(t))
	}
	env.evm.StateDB.AddLog(&types.Log{
		Address: common.Address(address),
		Topics:  ethTopics,
		Data:    data,
	})
}

func (env *environment) BlockTime() uint64 {
	return env.evm.Time.Uint64()
}

func (env *environment) BlockNumber() uint32 {
	return uint32(env.evm.BlockNumber.Uint64())
}

func (env *environment) Caller() thor.Address {
	return thor.Address(env.contract.Caller())
}

func (env *environment) To() thor.Address {
	return thor.Address(env.contract.Address())
}

func (env *environment) Stop(err error) {
	panic(&vmError{err})
}

type vmError struct {
	cause error
}

func (e *vmError) Error() string {
	return e.cause.Error()
}
