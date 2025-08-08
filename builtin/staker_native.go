// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/staker/validation"
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
				panic(err)
			}
			return []any{staked, weight}
		}},
		{"native_queuedStake", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			staked, weight, err := Staker.NativeMetered(env.State(), charger).QueuedStake()
			if err != nil {
				panic(err)
			}
			return []any{staked, weight}
		}},
		{"native_getValidation", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).GetValidation(thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}

			// IMPORTANT, DO NOT return zero value for pointer type, subsequent abi.EncodeOutput will panic due to call of reflect.ValueOf.
			if validator.IsEmpty() {
				return []any{thor.Address{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), validation.StatusUnknown, false, uint32(0), uint32(0), uint32(math.MaxUint32), uint32(0)}
			}
			exitBlock := uint32(math.MaxUint32)
			if validator.ExitBlock != nil {
				exitBlock = *validator.ExitBlock
			}
			return []any{
				validator.Endorsor,
				validator.LockedVET,
				validator.Weight,
				validator.QueuedVET,
				validator.Status,
				validator.Online,
				validator.Period,
				validator.StartBlock,
				exitBlock,
				validator.CompleteIterations,
			}
		}},
		{"native_getWithdrawable", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			amount, err := Staker.NativeMetered(env.State(), charger).GetWithdrawable(thor.Address(args.Validator), env.BlockContext().Number)
			if err != nil {
				panic(err)
			}
			return []any{amount}
		}},
		{"native_firstActive", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			first, err := Staker.NativeMetered(env.State(), charger).FirstActive()
			if err != nil {
				panic(err)
			}
			return []any{first}
		}},
		{"native_firstQueued", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			first, err := Staker.NativeMetered(env.State(), charger).FirstQueued()
			if err != nil {
				panic(err)
			}
			return []any{first}
		}},
		{"native_next", func(env *xenv.Environment) []any {
			var args struct {
				Prev common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			next, err := Staker.NativeMetered(env.State(), charger).Next(thor.Address(args.Prev))
			if err != nil {
				panic(err)
			}
			return []any{next}
		}},
		{"native_withdrawStake", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawStake(
				thor.Address(args.Validator),
				env.BlockContext().Number,
			)
			if err != nil {
				panic(err)
			}

			return []any{stake}
		}},
		{"native_addValidation", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
				Endorsor  common.Address
				Period    uint32
				Stake     *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			isPoSActive, err := Staker.NativeMetered(env.State(), charger).IsPoSActive()
			if err != nil {
				panic(err)
			}

			if !isPoSActive {
				charger.Charge(thor.SloadGas) // a.getEntry(ValidatorMaster)

				exists, endorsor, _, _, err := Authority.Native(env.State()).Get(thor.Address(args.Validator))
				if err != nil {
					panic(err)
				}
				if !exists {
					return []any{"staker:validator is not present in the authority"}
				}
				if thor.Address(args.Endorsor) != endorsor {
					return []any{"staker: invalid endorsor"} // TODO: check if this is correct
				}
			}

			ok, err := Staker.NativeMetered(env.State(), charger).
				AddValidation(
					thor.Address(args.Validator),
					thor.Address(args.Endorsor),
					args.Period,
					args.Stake,
				)
			if err != nil {
				panic(err)
			}

			if !ok {
				return []any{"staker: invalid period"}
			}

			return []any{""}
		}},
		{"native_signalExit", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				SignalExit(thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			return []any{}
		}},
		{"native_validateStakeChange", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
				Increase  *big.Int
				Decrease  *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			ok, err := Staker.NativeMetered(env.State(), charger).
				ValidateStakeChange(
					thor.Address(args.Validator),
					args.Increase,
					args.Decrease,
				)
			if err != nil {
				panic(err)
			}

			return []any{ok}
		}},
		{"native_increaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
				Amount    *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				IncreaseStake(
					thor.Address(args.Validator),
					args.Amount,
				)
			if err != nil {
				panic(err)
			}

			return []any{}
		}},
		{"native_decreaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
				Amount    *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				DecreaseStake(
					thor.Address(args.Validator),
					args.Amount,
				)
			if err != nil {
				panic(err)
			}

			return []any{}
		}},
		{"native_addDelegation", func(env *xenv.Environment) []any {
			var args struct {
				Validator  common.Address
				Stake      *big.Int
				Multiplier uint8
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegationID, err := Staker.NativeMetered(env.State(), charger).
				AddDelegation(
					thor.Address(args.Validator),
					args.Stake,
					args.Multiplier,
				)
			if err != nil {
				panic(err)
			}
			return []any{delegationID}
		}},

		{"native_withdrawDelegation", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawDelegation(args.DelegationID)
			if err != nil {
				panic(err)
			}

			return []any{stake}
		}},
		{"native_signalDelegationExit", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).SignalDelegationExit(args.DelegationID)
			if err != nil {
				panic(err)
			}

			return []any{}
		}},
		{"native_getDelegation", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegation, validation, err := Staker.NativeMetered(env.State(), charger).GetDelegation(args.DelegationID)
			if err != nil {
				panic(err)
			}

			// IMPORTANT, DO NOT return zero value for pointer type, subsequent abi.EncodeOutput will panic due to call of reflect.ValueOf.
			if delegation.IsEmpty() {
				return []any{thor.Address{}, big.NewInt(0), uint32(0), uint32(math.MaxUint32), uint8(0), false}
			}

			lastPeriod := uint32(math.MaxUint32)
			if delegation.LastIteration != nil {
				lastPeriod = *delegation.LastIteration
			}

			locked := delegation.Started(validation) && !delegation.Ended(validation)
			return []any{
				delegation.Validator,
				delegation.Stake,
				delegation.FirstIteration,
				lastPeriod,
				delegation.Multiplier,
				locked,
			}
		}},
		{"native_getDelegatorRewards", func(env *xenv.Environment) []any {
			var args struct {
				Validator     common.Address
				StakingPeriod uint32
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			reward, err := Staker.NativeMetered(env.State(), charger).GetDelegatorRewards(thor.Address(args.Validator), args.StakingPeriod)
			if err != nil {
				panic(err)
			}
			return []any{reward}
		}},
		{"native_getValidationTotals", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			totals, err := Staker.NativeMetered(env.State(), charger).GetValidationTotals(thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			return []any{
				totals.TotalLockedStake,
				totals.TotalLockedWeight,
				totals.TotalQueuedStake,
				totals.TotalQueuedWeight,
				totals.TotalExitingStake,
				totals.TotalExitingWeight,
			}
		}},
		{"native_getDelegatorContract", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			charger.Charge(thor.SloadGas)
			raw, err := Params.Native(env.State()).Get(thor.KeyDelegatorContractAddress)
			if err != nil {
				panic(err)
			}
			addr := thor.BytesToAddress(raw.Bytes())
			return []any{addr}
		}},
		{"native_getControlSwitches", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)
			charger.Charge(thor.SloadGas)

			switches, err := Params.Native(env.State()).Get(thor.KeyStakerSwitches)
			if err != nil {
				panic(err)
			}
			return []any{switches}
		}},
		{"native_getValidationNum", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			leaderGroupSize, queuedGroupSize, err := Staker.NativeMetered(env.State(), charger).GetValidationNum()
			if err != nil {
				panic(err)
			}
			return []any{leaderGroupSize, queuedGroupSize}
		}},
		{"native_issuance", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			totalStaked, _, err := Staker.NativeMetered(env.State(), charger).LockedVET()
			if err != nil {
				panic(err)
			}
			issuance, err := Energy.Native(env.State(), env.BlockContext().Time).CalculateRewards(totalStaked)
			if err != nil {
				panic(err)
			}
			return []any{issuance}
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
