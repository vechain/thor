// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) []any
	}{
		{"native_totalStake", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			staked, weight, err := Staker.NativeMetered(env.State(), charger).LockedVET()
			if err != nil {
				return []any{new(big.Int), new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{staked, weight, ""}
		}},
		{"native_queuedStake", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			staked, weight, err := Staker.NativeMetered(env.State(), charger).QueuedStake()
			if err != nil {
				return []any{new(big.Int), new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{staked, weight, ""}
		}},
		{"native_get", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).Get(thor.Address(args.ValidationID))
			if err != nil {
				return []any{
					thor.Address{},
					thor.Address{},
					big.NewInt(0),
					big.NewInt(0),
					staker.StatusUnknown,
					false,
					uint32(0),
					uint32(0),
					uint32(0),
					fmt.Sprintf("revert: %v", err),
				}
			}
			if validator.IsEmpty() {
				return []any{thor.Address{}, thor.Address{}, big.NewInt(0), big.NewInt(0), staker.StatusUnknown, false, uint32(0), uint32(0), uint32(0), ""}
			}
			exitBlock := uint32(math.MaxUint32)
			if validator.ExitBlock != nil {
				exitBlock = *validator.ExitBlock
			}
			return []any{
				thor.Address(args.ValidationID),
				validator.Endorsor,
				validator.LockedVET,
				validator.Weight,
				validator.Status,
				validator.Online,
				validator.Period,
				validator.StartBlock,
				exitBlock,
				"",
			}
		}},
		{"native_getWithdrawable", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			amount, err := Staker.NativeMetered(env.State(), charger).GetWithdrawable(thor.Address(args.ValidationID), env.BlockContext().Number)
			if err != nil {
				return []any{big.NewInt(0), fmt.Sprintf("revert: %v", err)}
			}
			return []any{amount, ""}
		}},
		{"native_firstActive", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			first, err := Staker.NativeMetered(env.State(), charger).FirstActive()
			if err != nil {
				return []any{thor.Address{}, fmt.Sprintf("revert: %v", err)}
			}
			return []any{first, ""}
		}},
		{"native_firstQueued", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			first, err := Staker.NativeMetered(env.State(), charger).FirstQueued()
			if err != nil {
				return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
			}
			return []any{first, ""}
		}},
		{"native_next", func(env *xenv.Environment) []any {
			var args struct {
				Prev common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			next, err := Staker.NativeMetered(env.State(), charger).Next(thor.Address(args.Prev))
			if err != nil {
				return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
			}
			return []any{next, ""}
		}},
		{"native_withdrawStake", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor     common.Address
				ValidationID common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawStake(
				thor.Address(args.Endorsor),
				thor.Address(args.ValidationID),
				env.BlockContext().Number,
			)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			return []any{stake, ""}
		}},
		{"native_addValidator", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor common.Address
				Node     common.Address
				Period   uint32
				Stake    *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			isPoSActive, err := Staker.NativeMetered(env.State(), charger).IsPoSActive()
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			if !isPoSActive {
				charger.Charge(thor.SloadGas) // a.getEntry(nodeMaster)

				exists, endorsor, _, _, err := Authority.Native(env.State()).Get(thor.Address(args.Node))
				if err != nil {
					return []any{fmt.Sprintf("revert: %v", err)}
				}
				if !exists {
					return []any{"revert: node is not present in the Authority"}
				}
				if thor.Address(args.Endorsor) != endorsor {
					return []any{"revert: endorsor is not present in the Authority"}
				}
			}

			err = Staker.NativeMetered(env.State(), charger).
				AddValidator(
					thor.Address(args.Endorsor),
					thor.Address(args.Node),
					args.Period,
					args.Stake,
				)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			return []any{""}
		}},
		{"native_signalExit", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor     common.Address
				ValidationID common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				SignalExit(
					thor.Address(args.Endorsor),
					thor.Address(args.ValidationID),
				)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}
			return []any{""}
		}},
		{"native_increaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor     common.Address
				ValidationID common.Address
				Amount       *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				IncreaseStake(
					thor.Address(args.Endorsor),
					thor.Address(args.ValidationID),
					args.Amount,
				)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			return []any{""}
		}},
		{"native_decreaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor     common.Address
				ValidationID common.Address
				Amount       *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				DecreaseStake(
					thor.Address(args.Endorsor),
					thor.Address(args.ValidationID),
					args.Amount,
				)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}
			return []any{""}
		}},
		{"native_addDelegation", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Address
				Stake        *big.Int
				Multiplier   uint8
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegationID, err := Staker.NativeMetered(env.State(), charger).
				AddDelegation(
					thor.Address(args.ValidationID),
					args.Stake,
					args.Multiplier,
				)
			if err != nil {
				return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
			}
			return []any{delegationID, ""}
		}},
		{"native_withdrawDelegation", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID common.Hash
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawDelegation(thor.Bytes32(args.DelegationID))
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			return []any{stake, ""}
		}},
		{"native_signalDelegationExit", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID common.Hash
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).SignalDelegationExit(thor.Bytes32(args.DelegationID))
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			return []any{""}
		}},
		{"native_getDelegation", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID common.Hash
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegation, validator, err := Staker.NativeMetered(env.State(), charger).GetDelegation(thor.Bytes32(args.DelegationID))
			if err != nil {
				return []any{thor.Bytes32{}, new(big.Int), uint32(0), uint32(0), uint8(0), false, false, fmt.Sprintf("revert: %v", err)}
			}

			lastPeriod := uint32(math.MaxUint32)
			if delegation.LastIteration != nil {
				lastPeriod = *delegation.LastIteration
			}

			locked := delegation.Started(validator) && !delegation.Ended(validator)
			return []any{
				delegation.ValidationID,
				delegation.Stake,
				delegation.FirstIteration,
				lastPeriod,
				delegation.Multiplier,
				locked, "",
			}
		}},
		{"native_getDelegatorsRewards", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID  common.Address
				StakingPeriod uint32
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			reward, err := Staker.NativeMetered(env.State(), charger).GetDelegatorRewards(thor.Address(args.ValidationID), args.StakingPeriod)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{reward, ""}
		}},
		{"native_getCompletedPeriods", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			periods, err := Staker.NativeMetered(env.State(), charger).GetCompletedPeriods(thor.Address(args.ValidationID))
			if err != nil {
				return []any{uint32(0), fmt.Sprintf("revert: %v", err)}
			}
			return []any{periods, ""}
		}},
		{"native_getDelegatorContract", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			charger.Charge(thor.SloadGas)
			raw, err := Params.Native(env.State()).Get(thor.KeyStargateContractAddress)
			if err != nil {
				return []any{thor.Address{}, fmt.Sprintf("revert: failed to get Stargate contract address: %v", err)}
			}
			addr := thor.BytesToAddress(raw.Bytes())
			return []any{addr, ""}
		}},
		{"native_getValidatorTotals", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			totals, err := Staker.NativeMetered(env.State(), charger).GetValidatorsTotals(thor.Address(args.ValidationID))
			if err != nil {
				return []any{new(big.Int), new(big.Int), new(big.Int), new(big.Int), fmt.Sprintf("revert: failed to get validators totals: %v", err)}
			}
			return []any{totals.TotalLockedStake, totals.TotalLockedWeight, totals.DelegationsLockedStake, totals.DelegationsLockedWeight, ""}
		}},
		{"native_getValidatorsNum", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			leaderGroupSize, queuedGroupSize, err := Staker.NativeMetered(env.State(), charger).GetValidatorsNum()
			if err != nil {
				return []any{new(big.Int), new(big.Int), fmt.Sprintf("revert: failed to get validators totals: %v", err)}
			}
			return []any{leaderGroupSize, queuedGroupSize, ""}
		}},
	}
	stakerAbi := Staker.NativeABI()
	for _, def := range defines {
		if method, found := stakerAbi.MethodByName(def.name); found {
			nativeMethods[methodKey{Staker.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
