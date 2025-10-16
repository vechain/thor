// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// NewCustomNet create custom network genesis.
func NewCustomNet(gen *CustomGenesis) (*Genesis, error) {
	if gen.Config != nil {
		if gen.Config.BlockInterval <= 1 {
			return nil, errors.New("BlockInterval can not be zero or one")
		}

		if gen.Config.EpochLength <= 1 {
			return nil, errors.New("EpochLength can not be zero or one")
		}

		thor.SetConfig(*gen.Config)
		thor.LockConfig()
	}

	launchTime := gen.LaunchTime
	if gen.GasLimit == 0 {
		gen.GasLimit = thor.InitialGasLimit
	}

	if gen.Params.ExecutorAddress != nil && len(gen.Executor.Approvers) > 0 {
		return nil, errors.New("can not specify both executorAddress and approvers")
	}

	executor := builtin.Executor.Address
	externalExecutor := false
	// if external executor address is specified, ignore gen.Executor.Approvers
	if gen.Params.ExecutorAddress != nil {
		executor = *gen.Params.ExecutorAddress
		externalExecutor = true
	}

	builder := new(Builder).
		Timestamp(launchTime).
		GasLimit(gen.GasLimit).
		ForkConfig(gen.ForkConfig).
		State(func(state *state.State) error {
			// alloc builtin contracts
			if err := state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes()); err != nil {
				return err
			}
			if !externalExecutor {
				if err := state.SetCode(builtin.Executor.Address, builtin.Executor.RuntimeBytecodes()); err != nil {
					return err
				}
			}

			tokenSupply := &big.Int{}
			energySupply := &big.Int{}
			for _, a := range gen.Accounts {
				if b := (*big.Int)(a.Balance); b != nil {
					if b.Sign() < 0 {
						return fmt.Errorf("%s: balance must be a non-negative integer", a.Address)
					}
					tokenSupply.Add(tokenSupply, b)
					if err := state.SetBalance(a.Address, b); err != nil {
						return err
					}
					if err := state.SetEnergy(a.Address, &big.Int{}, launchTime); err != nil {
						return err
					}
				}
				if e := (*big.Int)(a.Energy); e != nil {
					if e.Sign() < 0 {
						return fmt.Errorf("%s: energy must be a non-negative integer", a.Address)
					}
					energySupply.Add(energySupply, e)
					if err := state.SetEnergy(a.Address, e, launchTime); err != nil {
						return err
					}
				}
				if len(a.Code) > 0 {
					code, err := hexutil.Decode(a.Code)
					if err != nil {
						return fmt.Errorf("invalid contract code for address: %s", a.Address)
					}
					if err := state.SetCode(a.Address, code); err != nil {
						return err
					}
				}
				if len(a.Storage) > 0 {
					for k, v := range a.Storage {
						state.SetStorage(a.Address, thor.MustParseBytes32(k), v)
					}
				}
			}

			return builtin.Energy.Native(state, launchTime).SetInitialSupply(tokenSupply, energySupply)
		})

	///// initialize builtin contracts

	// initialize params
	bgp := (*big.Int)(gen.Params.BaseGasPrice)
	if bgp != nil {
		if bgp.Sign() < 0 {
			return nil, errors.New("baseGasPrice must be a non-negative integer")
		}
	} else {
		bgp = thor.InitialBaseGasPrice
	}

	r := (*big.Int)(gen.Params.RewardRatio)
	if r != nil {
		if r.Sign() < 0 {
			return nil, errors.New("rewardRatio must be a non-negative integer")
		}
	} else {
		r = thor.InitialRewardRatio
	}

	e := (*big.Int)(gen.Params.ProposerEndorsement)
	if e != nil {
		if e.Sign() < 0 {
			return nil, errors.New("proposerEndorsement must a non-negative integer")
		}
	} else {
		e = thor.InitialProposerEndorsement
	}

	data := mustEncodeInput(builtin.Params.ABI, "set", thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), thor.Address{})

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyRewardRatio, r)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyLegacyTxBaseGasPrice, bgp)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyProposerEndorsement, e)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	if m := gen.Params.MaxBlockProposers; m != nil {
		if *m == uint64(0) {
			return nil, errors.New("maxBlockProposers must a non-negative integer")
		}
		data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyMaxBlockProposers, new(big.Int).SetUint64(*m))
		builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)
	}

	if len(gen.Authority) == 0 {
		return nil, errors.New("at least one authority node")
	}
	// add initial authority nodes
	for _, anode := range gen.Authority {
		data = mustEncodeInput(builtin.Authority.ABI, "add", anode.MasterAddress, anode.EndorsorAddress, anode.Identity)
		builder.Call(tx.NewClause(&builtin.Authority.Address).WithData(data), executor)
	}

	if !externalExecutor {
		for _, approver := range gen.Executor.Approvers {
			data = mustEncodeInput(builtin.Executor.ABI, "addApprover", approver.Address, approver.Identity)
			// using builtin.Executor.Address guarantees the execution of this clause
			builder.Call(tx.NewClause(&builtin.Executor.Address).WithData(data), builtin.Executor.Address)
		}
	}

	if gen.ForkConfig.HAYABUSA == 0 {
		for _, authority := range gen.Authority {
			data = mustEncodeInput(builtin.Staker.ABI, "addValidation", authority.MasterAddress, thor.HighStakingPeriod())
			builder.Call(tx.NewClause(&builtin.Staker.Address).WithData(data).WithValue(staker.MinStake), authority.EndorsorAddress)
		}
	}

	builder.PostCallState(func(state *state.State) error {
		// if hayabusa starts from genesis, skip the transition period and directly promote queued validators to active
		if gen.ForkConfig.HAYABUSA == 0 {
			stk := staker.New(builtin.Staker.Address, state, params.New(builtin.Params.Address, state), nil)
			_, err := stk.Housekeep(0)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if len(gen.ExtraData) > 0 {
		var extra [28]byte
		copy(extra[:], gen.ExtraData)
		builder.ExtraData(extra)
	}

	id, err := builder.ComputeID()
	if err != nil {
		panic(err)
	}
	return &Genesis{builder, id, "customnet"}, nil
}
