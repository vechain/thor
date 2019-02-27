// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

// PrivateGenesis is user customized genesis
type PrivateGenesis struct {
	LaunchTime     uint64          `json:"launchTime"`
	GasLimit       uint64          `json:"gaslimit"`
	ExtraData      string          `json:"extraData"`
	Accounts       []Account       `json:"accounts"`
	AuthorityNodes []AuthorityNode `json:"authority-nodes"`
	Params         Params          `json:"params"`
}

// NewPrivateNet create mainnet genesis.
func NewPrivateNet(gen *PrivateGenesis) (*Genesis, error) {
	launchTime := gen.LaunchTime

	if gen.GasLimit < 0 {
		return nil, errors.New("gasLimit must not be 0")
	}
	switch gen.Params.Executor.Type {
	case "contract":
		if len(gen.Params.Executor.Approvers) == 0 {
			return nil, errors.New("at least one approver for executor contract")
		}
		break
	case "address":
		if gen.Params.Executor.Address == nil {
			return nil, errors.New("executor address must be set")
		}
		break
	default:
		return nil, fmt.Errorf("unrecognized type of executor: %s", gen.Params.Executor.Type)
	}

	builder := new(Builder).
		Timestamp(launchTime).
		GasLimit(thor.InitialGasLimit).
		State(func(state *state.State) error {
			// alloc precompiled contracts
			for addr := range vm.PrecompiledContractsByzantium {
				state.SetCode(thor.Address(addr), emptyRuntimeBytecode)
			}

			// alloc builtin contracts
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
			state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes())
			state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
			state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes())

			if gen.Params.Executor.Type == "contract" {
				state.SetCode(builtin.Executor.Address, builtin.Executor.RuntimeBytecodes())
			}

			tokenSupply := &big.Int{}
			energySupply := &big.Int{}
			for _, a := range gen.Accounts {
				if a.Balance == nil {
					return fmt.Errorf("%s: balance must be set", a.Address)
				}
				if a.Balance.Sign() < 1 {
					return fmt.Errorf("%s: balance must be a non-zero integer", a.Address)
				}
				if a.Balance.Sign() < 1 {
					return fmt.Errorf("%s: balance must be a non-zero integer", a.Address)
				}

				tokenSupply.Add(tokenSupply, a.Balance)
				state.SetBalance(a.Address, a.Balance)
				if a.Energy != nil {
					if a.Energy.Sign() < 1 {
						return fmt.Errorf("%s: balance must be a non-zero integer", a.Address)
					}
					energySupply.Add(energySupply, a.Energy)
					state.SetEnergy(a.Address, a.Energy, launchTime)
				}
				if len(a.Code) > 0 {
					code, err := hexutil.Decode(a.Code)
					if err != nil {
						return fmt.Errorf("invalid contract code for address: %s", a.Address)
					}
					state.SetCode(a.Address, code)
				}
				if len(a.Storage) > 0 {
					for k, v := range a.Storage {
						state.SetStorage(a.Address, thor.MustParseBytes32(k), v)
					}
				}
			}

			builtin.Energy.Native(state, launchTime).SetInitialSupply(tokenSupply, energySupply)
			return nil
		})

	///// initialize builtin contracts

	// initialize params
	if gen.Params.BaseGasPrice.Sign() < 1 {
		return nil, errors.New("baseGasPrice must be a non-zero integer")
	}
	if gen.Params.RewardRatio.Sign() < 1 {
		return nil, errors.New("rewardRatio must be a non-zero integer")
	}
	if gen.Params.ProposerEndorsement.Sign() < 1 {
		return nil, errors.New("proposerEndorsement must be a non-zero integer")
	}

	var executor thor.Address
	if gen.Params.Executor.Type == "contract" {
		executor = builtin.Executor.Address
	} else {
		executor = *gen.Params.Executor.Address
	}

	data := mustEncodeInput(builtin.Params.ABI, "set", thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), thor.Address{})

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyRewardRatio, gen.Params.RewardRatio)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyBaseGasPrice, gen.Params.BaseGasPrice)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyProposerEndorsement, gen.Params.ProposerEndorsement)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	if len(gen.AuthorityNodes) == 0 {
		return nil, errors.New("at least one authority node")
	}
	// add initial authority nodes
	for _, anode := range gen.AuthorityNodes {
		data := mustEncodeInput(builtin.Authority.ABI, "add", anode.MasterAddress, anode.EndorsorAddress, anode.Identity)
		builder.Call(tx.NewClause(&builtin.Authority.Address).WithData(data), executor)
	}

	if gen.Params.Executor.Type == "contract" {
		// add initial approvers
		for _, approver := range gen.Params.Executor.Approvers {
			data := mustEncodeInput(builtin.Executor.ABI, "addApprover", approver.Address, approver.Identity)
			builder.Call(tx.NewClause(&builtin.Executor.Address).WithData(data), executor)
		}
	}

	if len(gen.ExtraData) > 0 {
		var extra [28]byte
		copy(extra[:], "Salute & Respect, Ethereum!")
		builder.ExtraData(extra)
	}

	id, err := builder.ComputeID()
	if err != nil {
		panic(err)
	}
	return &Genesis{builder, id, "privatenet"}, nil
}

// Account is the account will set to the genesis block
type Account struct {
	Address thor.Address            `json:"address"`
	Balance *big.Int                `json:"balance"`
	Energy  *big.Int                `json:"energy,omitempty"`
	Code    string                  `json:"code,omitempty"`
	Storage map[string]thor.Bytes32 `json:"storage,omitempty"`
}

// AuthorityNode is the authority node info
type AuthorityNode struct {
	MasterAddress   thor.Address `json:"masterAddress"`
	EndorsorAddress thor.Address `json:"endorsorAddress"`
	Identity        thor.Bytes32 `json:"identity"`
}

// Executor is the params for executor info
type Executor struct {
	Type      string        `json:"type"`
	Address   *thor.Address `json:"address,omitempty"`
	Approvers []Approver    `json:"approvers,omitempty"`
}

// Approver is the approver info for executor contract
type Approver struct {
	Address  thor.Address `json:"address"`
	Identity thor.Bytes32 `json:"identity"`
}

// Params means the chain params for param contract
type Params struct {
	RewardRatio         *big.Int `json:"rewardRatio"`
	BaseGasPrice        *big.Int `json:"baseGasPrice"`
	ProposerEndorsement *big.Int `json:"proposerEndorsement"`
	Executor            Executor `json:"executor"`
}
