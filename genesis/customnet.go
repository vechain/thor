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

	// When a Params.ExecutorAddress is set, the gen.Executor.Approvers cannot be set by the genesis
	// as the ExecutorAddress can be a contract or an EOA
	if gen.Params.ExecutorAddress != nil && len(gen.Executor.Approvers) > 0 {
		return nil, errors.New("can not specify both executorAddress and approvers")
	}

	executor := builtin.Executor.Address
	externalExecutor := false

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
			if isHayabusaGenesis(gen) {
				if err := state.SetCode(builtin.Staker.Address, builtin.Staker.RuntimeBytecodes()); err != nil {
					return err
				}
			}
			if deployBuiltinStargate(gen) {
				if err := state.SetCode(builtin.ClockLib.Address, builtin.ClockLib.RuntimeBytecodes()); err != nil {
					return err
				}
				if err := state.SetCode(builtin.LevelsLib.Address, builtin.LevelsLib.RuntimeBytecodes()); err != nil {
					return err
				}
				if err := state.SetCode(builtin.MintingLogicLib.Address, builtin.MintingLogicLib.RuntimeBytecodes()); err != nil {
					return err
				}
				if err := state.SetCode(builtin.SettingsLib.Address, builtin.SettingsLib.RuntimeBytecodes()); err != nil {
					return err
				}
				if err := state.SetCode(builtin.TokenLib.Address, builtin.TokenLib.RuntimeBytecodes()); err != nil {
					return err
				}
				if err := state.SetCode(builtin.TokenManagerLib.Address, builtin.TokenManagerLib.RuntimeBytecodes()); err != nil {
					return err
				}
				if err := state.SetCode(builtin.Stargate.Address, builtin.Stargate.RuntimeBytecodes()); err != nil {
					return err
				}
				if err := state.SetCode(builtin.StargateNFT.Address, builtin.StargateNFT.RuntimeBytecodes()); err != nil {
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

	if isHayabusaGenesis(gen) {
		// auth nodes are now validators
		for _, authority := range gen.Authority {
			data = mustEncodeInput(builtin.Staker.ABI, "addValidation", authority.MasterAddress, thor.HighStakingPeriod())
			builder.Call(tx.NewClause(&builtin.Staker.Address).WithData(data).WithValue(staker.MinStake), authority.EndorsorAddress)
		}
	}

	if deployBuiltinStargate(gen) {
		// initialize stargate nft
		blocksPerDay := 6 * 60 * 24
		strengthVetAmountRequiredToStake, _ := new(big.Int).SetString("1000000000000000000000000", 10)
		thunderVetAmountRequiredToStake, _ := new(big.Int).SetString("5000000000000000000000000", 10)
		mjolnirVetAmountRequiredToStake, _ := new(big.Int).SetString("15000000000000000000000000", 10)
		vethorXVetAmountRequiredToStake, _ := new(big.Int).SetString("600000000000000000000000", 10)
		strengthXVetAmountRequiredToStake, _ := new(big.Int).SetString("1600000000000000000000000", 10)
		thunderXVetAmountRequiredToStake, _ := new(big.Int).SetString("5600000000000000000000000", 10)
		mjolnirXVetAmountRequiredToStake, _ := new(big.Int).SetString("15600000000000000000000000", 10)
		dawnVetAmountRequiredToStake, _ := new(big.Int).SetString("10000000000000000000000", 10)
		lighningVetAmountRequiredToStake, _ := new(big.Int).SetString("50000000000000000000000", 10)
		flashVetAmountRequiredToStake, _ := new(big.Int).SetString("200000000000000000000000", 10)
		data = mustEncodeInput(builtin.StargateNFT.ABI, "initialize", builtin.StargatNFTInitializeV1Params{
			TokenCollectionName:   "StarGate Delegator Token",
			TokenCollectionSymbol: "SDT",
			BaseTokenURI:          "ipfs://bafybeibmpgruasnoqgyemcprpkygtelvxl3b5d2bf5aqqciw6dds33yw7y/metadata/",
			Admin:                 executor,
			Upgrader:              executor,
			Pauser:                executor,
			LevelOperator:         executor,
			LegacyNodes:           executor, // We dont care about the legacy nodes right now
			StargateDelegation:    executor, // We dont care about the stargate delegation right now
			VthoToken:             builtin.Energy.Address,
			LegacyLastTokenId:     10000,
			LevelsAndSupplies: []builtin.LevelAndSupply{
				{
					Level: builtin.Level{
						ID:                       1,
						Name:                     "Strength",
						IsX:                      false,
						VetAmountRequiredToStake: strengthVetAmountRequiredToStake,
						MaturityBlocks:           uint64(blocksPerDay * 30),
						ScaledRewardFactor:       150,
					},
					Cap:               1382,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       2,
						Name:                     "Thunder",
						IsX:                      false,
						VetAmountRequiredToStake: thunderVetAmountRequiredToStake,
						MaturityBlocks:           uint64(blocksPerDay * 45),
						ScaledRewardFactor:       250,
					},
					Cap:               234,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       3,
						Name:                     "Mjolnir",
						IsX:                      false,
						VetAmountRequiredToStake: mjolnirVetAmountRequiredToStake,
						MaturityBlocks:           uint64(blocksPerDay * 60),
						ScaledRewardFactor:       350,
					},
					Cap:               13,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       4,
						Name:                     "VeThorX",
						IsX:                      true,
						VetAmountRequiredToStake: vethorXVetAmountRequiredToStake,
						MaturityBlocks:           0,
						ScaledRewardFactor:       200,
					},
					Cap:               0,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       5,
						Name:                     "StrengthX",
						IsX:                      true,
						VetAmountRequiredToStake: strengthXVetAmountRequiredToStake,
						MaturityBlocks:           0,
						ScaledRewardFactor:       300,
					},
					Cap:               0,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       6,
						Name:                     "ThunderX",
						IsX:                      true,
						VetAmountRequiredToStake: thunderXVetAmountRequiredToStake,
						MaturityBlocks:           0,
						ScaledRewardFactor:       400,
					},
					Cap:               0,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       7,
						Name:                     "MjolnirX",
						IsX:                      true,
						VetAmountRequiredToStake: mjolnirXVetAmountRequiredToStake,
						MaturityBlocks:           0,
						ScaledRewardFactor:       500,
					},
					Cap:               0,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       8,
						Name:                     "Dawn",
						IsX:                      false,
						VetAmountRequiredToStake: dawnVetAmountRequiredToStake,
						MaturityBlocks:           uint64(blocksPerDay * 2),
						ScaledRewardFactor:       100,
					},
					Cap:               500000,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       9,
						Name:                     "Lighning",
						IsX:                      false,
						VetAmountRequiredToStake: lighningVetAmountRequiredToStake,
						MaturityBlocks:           uint64(blocksPerDay * 5),
						ScaledRewardFactor:       115,
					},
					Cap:               100000,
					CirculatingSupply: 0,
				},
				{
					Level: builtin.Level{
						ID:                       10,
						Name:                     "Flash",
						IsX:                      false,
						VetAmountRequiredToStake: flashVetAmountRequiredToStake,
						MaturityBlocks:           uint64(blocksPerDay * 15),
						ScaledRewardFactor:       130,
					},
					Cap:               25000,
					CirculatingSupply: 0,
				},
			},
		})
		builder.Call(tx.NewClause(&builtin.StargateNFT.Address).WithData(data), executor)

		tokenIds := []uint8{8, 9, 10, 1, 2, 3}
		dawnBoostPricePerBlock, _ := new(big.Int).SetString("539351851851852n", 10)
		lighningBoostPricePerBlock, _ := new(big.Int).SetString("2870370370370370n", 10)
		flashBoostPricePerBlock, _ := new(big.Int).SetString("12523148148148100n", 10)
		strengthBoostPricePerBlock, _ := new(big.Int).SetString("75925925925925900n", 10)
		thunderBoostPricePerBlock, _ := new(big.Int).SetString("530092592592593000n", 10)
		mjolnirBoostPricePerBlock, _ := new(big.Int).SetString("1995370370370370000n", 10)

		boostPricesPerBlock := []big.Int{
			*dawnBoostPricePerBlock,
			*lighningBoostPricePerBlock,
			*flashBoostPricePerBlock,
			*strengthBoostPricePerBlock,
			*thunderBoostPricePerBlock,
			*mjolnirBoostPricePerBlock,
		}

		data = mustEncodeInput(builtin.StargateNFT.ABI, "initializeV3", builtin.Stargate.Address, tokenIds, boostPricesPerBlock)
		builder.Call(tx.NewClause(&builtin.StargateNFT.Address).WithData(data), executor)

		data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyDelegatorContractAddress, builtin.Stargate.Address)
		builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)
	}

	builder.PostCallState(func(state *state.State) error {
		if isHayabusaGenesis(gen) {
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

func isHayabusaGenesis(gen *CustomGenesis) bool {
	return gen.ForkConfig.HAYABUSA == 0 && gen.Config != nil && gen.Config.HayabusaTP != nil && *gen.Config.HayabusaTP == 0
}

func deployBuiltinStargate(gen *CustomGenesis) bool {
	return gen.Config != nil && gen.Config.BuiltInStargate != nil && *gen.Config.BuiltInStargate == 0
}
