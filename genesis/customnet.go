// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// CustomGenesis is user customized genesis
type CustomGenesis struct {
	LaunchTime uint64           `json:"launchTime"`
	GasLimit   uint64           `json:"gaslimit"`
	ExtraData  string           `json:"extraData"`
	Accounts   []Account        `json:"accounts"`
	Authority  []Authority      `json:"authority"`
	Params     Params           `json:"params"`
	Executor   Executor         `json:"executor"`
	ForkConfig *thor.ForkConfig `json:"forkConfig"`
}

// NewCustomNet create custom network genesis.
func NewCustomNet(gen *CustomGenesis) (*Genesis, error) {
	launchTime := gen.LaunchTime

	if gen.GasLimit == 0 {
		gen.GasLimit = thor.InitialGasLimit
	}
	var executor thor.Address
	if gen.Params.ExecutorAddress != nil {
		executor = *gen.Params.ExecutorAddress
	} else {
		executor = builtin.Executor.Address
	}

	builder := new(Builder).
		Timestamp(launchTime).
		GasLimit(gen.GasLimit).
		ForkConfig(*gen.ForkConfig).
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

			if len(gen.Executor.Approvers) > 0 {
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

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyBaseGasPrice, bgp)
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
		data := mustEncodeInput(builtin.Authority.ABI, "add", anode.MasterAddress, anode.EndorsorAddress, anode.Identity)
		builder.Call(tx.NewClause(&builtin.Authority.Address).WithData(data), executor)
	}

	if len(gen.Executor.Approvers) > 0 {
		// add initial approvers
		for _, approver := range gen.Executor.Approvers {
			data := mustEncodeInput(builtin.Executor.ABI, "addApprover", approver.Address, approver.Identity)
			builder.Call(tx.NewClause(&builtin.Executor.Address).WithData(data), executor)
		}
	}

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

// Account is the account will set to the genesis block
type Account struct {
	Address thor.Address            `json:"address"`
	Balance *HexOrDecimal256        `json:"balance"`
	Energy  *HexOrDecimal256        `json:"energy"`
	Code    string                  `json:"code"`
	Storage map[string]thor.Bytes32 `json:"storage"`
}

// Authority is the authority node info
type Authority struct {
	MasterAddress   thor.Address `json:"masterAddress"`
	EndorsorAddress thor.Address `json:"endorsorAddress"`
	Identity        thor.Bytes32 `json:"identity"`
}

// Executor is the params for executor info
type Executor struct {
	Approvers []Approver `json:"approvers"`
}

// Approver is the approver info for executor contract
type Approver struct {
	Address  thor.Address `json:"address"`
	Identity thor.Bytes32 `json:"identity"`
}

// Params means the chain params for params contract
type Params struct {
	RewardRatio         *HexOrDecimal256 `json:"rewardRatio"`
	BaseGasPrice        *HexOrDecimal256 `json:"baseGasPrice"`
	ProposerEndorsement *HexOrDecimal256 `json:"proposerEndorsement"`
	ExecutorAddress     *thor.Address    `json:"executorAddress"`
	MaxBlockProposers   *uint64          `json:"maxBlockProposers"`
}

// hexOrDecimal256 marshals big.Int as hex or decimal.
// Copied from go-ethereum/common/math and implement json. Marshaler
type HexOrDecimal256 math.HexOrDecimal256

// UnmarshalJSON implements the json.Unmarshaler interface.
func (i *HexOrDecimal256) UnmarshalJSON(input []byte) error {
	var hex string
	if err := json.Unmarshal(input, &hex); err != nil {
		if err = (*big.Int)(i).UnmarshalJSON(input); err != nil {
			return err
		}
		return nil
	}
	bigint, ok := math.ParseBig256(hex)
	if !ok {
		return fmt.Errorf("invalid hex or decimal integer %q", input)
	}
	*i = HexOrDecimal256(*bigint)
	return nil
}

// MarshalJSON implements the json.Marshaler interface.
func (i *HexOrDecimal256) MarshalJSON() ([]byte, error) {
	return (*math.HexOrDecimal256)(i).MarshalText()
}
