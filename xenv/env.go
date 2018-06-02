// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package xenv

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

// BlockContext block context.
type BlockContext struct {
	Beneficiary thor.Address
	Signer      thor.Address
	Number      uint32
	Time        uint64
	GasLimit    uint64
	TotalScore  uint64
}

// TransactionContext transaction context.
type TransactionContext struct {
	ID         thor.Bytes32
	Origin     thor.Address
	GasPrice   *big.Int
	ProvedWork *big.Int
}

type vmError struct {
	cause error
}

// Environment an env to execute native method.
type Environment struct {
	abi      *abi.Method
	seeker   *chain.Seeker
	state    *state.State
	blockCtx *BlockContext
	txCtx    *TransactionContext
	evm      *evm.EVM
	contract *evm.Contract
}

// New create a new env.
func New(
	abi *abi.Method,
	seeker *chain.Seeker,
	state *state.State,
	blockCtx *BlockContext,
	txCtx *TransactionContext,
	evm *evm.EVM,
	contract *evm.Contract,
) *Environment {
	return &Environment{
		abi:      abi,
		seeker:   seeker,
		state:    state,
		blockCtx: blockCtx,
		txCtx:    txCtx,
		evm:      evm,
		contract: contract,
	}
}

func (env *Environment) Seeker() *chain.Seeker                   { return env.seeker }
func (env *Environment) State() *state.State                     { return env.state }
func (env *Environment) TransactionContext() *TransactionContext { return env.txCtx }
func (env *Environment) BlockContext() *BlockContext             { return env.blockCtx }
func (env *Environment) Caller() thor.Address                    { return thor.Address(env.contract.Caller()) }
func (env *Environment) To() thor.Address                        { return thor.Address(env.contract.Address()) }

func (env *Environment) UseGas(gas uint64) {
	if !env.contract.UseGas(gas) {
		panic(&vmError{evm.ErrOutOfGas})
	}
}

func (env *Environment) ParseArgs(val interface{}) {
	if err := env.abi.DecodeInput(env.contract.Input, val); err != nil {
		// as vm error
		panic(&vmError{errors.WithMessage(err, "decode native input")})
	}
}

func (env *Environment) Require(cond bool) {
	if !cond {
		panic(&vmError{evm.ErrExecutionReverted()})
	}
}

func (env *Environment) Log(abi *abi.Event, address thor.Address, topics []thor.Bytes32, args ...interface{}) {
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

func (env *Environment) Stop(vmerr error) {
	panic(&vmError{vmerr})
}

func (env *Environment) Call(proc func(env *Environment) []interface{}, readonly bool) func() ([]byte, error) {
	return func() (data []byte, err error) {
		if readonly && !env.abi.Const() {
			return nil, evm.ErrWriteProtection()
		}

		if env.contract.Value().Sign() != 0 {
			// reject value transfer on call
			return nil, evm.ErrExecutionReverted()
		}

		defer func() {
			if e := recover(); e != nil {
				if rec, ok := e.(*vmError); ok {
					err = rec.cause
				} else {
					panic(e)
				}
			}
		}()
		output := proc(env)
		data, err = env.abi.EncodeOutput(output...)
		if err != nil {
			panic(errors.WithMessage(err, "encode native output"))
		}
		return
	}
}
