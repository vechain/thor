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
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) ([]any, error)
	}{
		{"native_totalStake", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			staked, weight, err := Staker.NativeMetered(env.State(), charger).LockedStake()
			if err != nil {
				return nil, err
			}
			return []any{
				staker.ToWei(staked),
				staker.ToWei(weight),
			}, nil
		}},
		{"native_queuedStake", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			staked, err := Staker.NativeMetered(env.State(), charger).QueuedStake()
			if err != nil {
				return nil, err
			}
			return []any{
				staker.ToWei(staked),
			}, nil
		}},
		{"native_getValidation", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).GetValidation(thor.Address(args.Validator))
			if err != nil {
				return nil, err
			}

			// IMPORTANT, DO NOT return zero value for pointer type
			// Subsequent abi.EncodeOutput will panic due to call of reflect.ValueOf.
			if validator == nil {
				return []any{
					thor.Address{},
					big.NewInt(0),
					big.NewInt(0),
					big.NewInt(0),
					validation.StatusUnknown,
					uint32(math.MaxUint32),
					uint32(0),
					uint32(0),
					uint32(math.MaxUint32),
					uint32(0),
				}, nil
			}
			exitBlock := uint32(math.MaxUint32)
			if validator.ExitBlock != nil {
				exitBlock = *validator.ExitBlock
			}

			offlineBlock := uint32(math.MaxUint32)
			if validator.OfflineBlock != nil {
				offlineBlock = *validator.OfflineBlock
			}

			completeIterations, err := validator.CompletedIterations(env.BlockContext().Number)
			if err != nil {
				return nil, err
			}
			return []any{
				validator.Endorser,
				staker.ToWei(validator.LockedVET),
				staker.ToWei(validator.Weight),
				staker.ToWei(validator.QueuedVET),
				validator.Status,
				offlineBlock,
				validator.Period,
				validator.StartBlock,
				exitBlock,
				completeIterations,
			}, nil
		}},
		{"native_getWithdrawable", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			amount, err := Staker.NativeMetered(env.State(), charger).GetWithdrawable(thor.Address(args.Validator), env.BlockContext().Number)
			if err != nil {
				return nil, err
			}
			return []any{staker.ToWei(amount)}, nil
		}},
		{"native_firstActive", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			first, err := Staker.NativeMetered(env.State(), charger).FirstActive()
			if err != nil {
				return nil, err
			}

			return []any{first}, nil
		}},
		{"native_firstQueued", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			first, err := Staker.NativeMetered(env.State(), charger).FirstQueued()
			if err != nil {
				return nil, err
			}

			return []any{first}, nil
		}},
		{"native_next", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Prev common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			next, err := Staker.NativeMetered(env.State(), charger).Next(thor.Address(args.Prev))
			if err != nil {
				return nil, err
			}

			return []any{next}, nil
		}},
		{"native_withdrawStake", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
				Endorser  common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawStake(
				thor.Address(args.Validator),
				thor.Address(args.Endorser),
				env.BlockContext().Number,
			)
			if err != nil {
				return nil, err
			}

			return []any{staker.ToWei(stake)}, nil
		}},
		{"native_addValidation", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
				Endorser  common.Address
				Period    uint32
				Stake     *big.Int
			}
			env.ParseArgs(&args)
			val := thor.Address(args.Validator)
			end := thor.Address(args.Endorser)
			charger := gascharger.New(env)

			isPoSActive, err := Staker.NativeMetered(env.State(), charger).IsPoSActive()
			if err != nil {
				return nil, err
			}

			if !isPoSActive {
				exists, endorser, _, _, err := Authority.Native(env.State()).Get(thor.Address(args.Validator))
				if err != nil {
					return nil, err
				}
				if !exists {
					return nil, staker.NewReverts("authority required in transition period")
				}
				if thor.Address(args.Endorser) != endorser {
					return nil, staker.NewReverts("endorser required")
				}
			}

			err = Staker.NativeMetered(env.State(), charger).
				AddValidation(
					val,
					end,
					args.Period,
					staker.ToVET(args.Stake), // convert from wei to VET
				)
			if err != nil {
				return nil, err
			}

			return []any{}, nil
		}},
		{"native_signalExit", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
				Endorser  common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				SignalExit(
					thor.Address(args.Validator),
					thor.Address(args.Endorser),
					env.BlockContext().Number,
				)
			if err != nil {
				return nil, err
			}
			return []any{}, nil
		}},
		{"native_increaseStake", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
				Endorser  common.Address
				Amount    *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				IncreaseStake(
					thor.Address(args.Validator),
					thor.Address(args.Endorser),
					staker.ToVET(args.Amount), // convert from wei to VET
				)
			if err != nil {
				return nil, err
			}

			return []any{}, nil
		}},
		{"native_setBeneficiary", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator   common.Address
				Endorser    common.Address
				Beneficiary common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				SetBeneficiary(
					thor.Address(args.Validator),
					thor.Address(args.Endorser),
					thor.Address(args.Beneficiary),
				)
			if err != nil {
				return nil, err
			}
			return []any{}, nil
		}},
		{"native_decreaseStake", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
				Endorser  common.Address
				Amount    *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).
				DecreaseStake(
					thor.Address(args.Validator),
					thor.Address(args.Endorser),
					staker.ToVET(args.Amount), // convert from wei to VET,
				)
			if err != nil {
				return nil, err
			}
			return []any{}, nil
		}},
		{"native_addDelegation", func(env *xenv.Environment) ([]any, error) {
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
					staker.ToVET(args.Stake), // convert from wei to VET,
					args.Multiplier,
					env.BlockContext().Number,
				)
			if err != nil {
				return nil, err
			}
			return []any{delegationID}, nil
		}},
		{"native_withdrawDelegation", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawDelegation(args.DelegationID, env.BlockContext().Number)
			if err != nil {
				return nil, err
			}

			return []any{staker.ToWei(stake)}, nil
		}},
		{"native_signalDelegationExit", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := Staker.NativeMetered(env.State(), charger).SignalDelegationExit(args.DelegationID, env.BlockContext().Number)
			if err != nil {
				return nil, err
			}

			return []any{}, nil
		}},
		{"native_getDelegation", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegation, validation, err := Staker.NativeMetered(env.State(), charger).GetDelegation(args.DelegationID)
			if err != nil {
				return nil, err
			}

			// IMPORTANT, DO NOT return zero value for pointer type
			// Subsequent abi.EncodeOutput will panic due to call of reflect.ValueOf.
			if delegation == nil {
				return []any{thor.Address{}, big.NewInt(0), uint8(0), false, uint32(0), uint32(math.MaxUint32)}, nil
			}

			lastPeriod := uint32(math.MaxUint32)
			if delegation.LastIteration != nil {
				lastPeriod = *delegation.LastIteration
			}
			isLocked, err := delegation.IsLocked(validation, env.BlockContext().Number)
			if err != nil {
				return nil, err
			}

			return []any{
				delegation.Validation,
				staker.ToWei(delegation.Stake),
				delegation.Multiplier,
				isLocked,
				delegation.FirstIteration,
				lastPeriod,
			}, nil
		}},
		{"native_getDelegatorsRewards", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator     common.Address
				StakingPeriod uint32
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			reward, err := Staker.NativeMetered(env.State(), charger).GetDelegatorRewards(thor.Address(args.Validator), args.StakingPeriod)
			if err != nil {
				return nil, err
			}
			return []any{reward}, nil
		}},
		{"native_getDelegatorContract", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			charger.Charge(thor.SloadGas)
			raw, err := Params.Native(env.State()).Get(thor.KeyDelegatorContractAddress)
			if err != nil {
				return nil, err
			}
			addr := thor.BytesToAddress(raw.Bytes())
			return []any{addr}, nil
		}},
		{"native_getValidationTotals", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			totals, err := Staker.NativeMetered(env.State(), charger).GetValidationTotals(thor.Address(args.Validator))
			if err != nil {
				return nil, err
			}
			if totals == nil {
				return []any{
					new(big.Int),
					new(big.Int),
					new(big.Int),
					new(big.Int),
					new(big.Int),
				}, nil
			}
			return []any{
				staker.ToWei(totals.TotalLockedStake),
				staker.ToWei(totals.TotalLockedWeight),
				staker.ToWei(totals.TotalQueuedStake),
				staker.ToWei(totals.TotalExitingStake),
				staker.ToWei(totals.NextPeriodWeight),
			}, nil
		}},
		{"native_getValidationsNum", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			leaderGroupSize, queuedGroupSize, err := Staker.NativeMetered(env.State(), charger).GetValidationsNum()
			if err != nil {
				return nil, err
			}
			return []any{leaderGroupSize, queuedGroupSize}, nil
		}},
		{"native_issuance", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			staker := Staker.NativeMetered(env.State(), charger)
			charger.Charge(thor.SloadGas)
			issuance, err := Energy.Native(env.State(), env.BlockContext().Time).CalculateRewards(staker)
			if err != nil {
				return nil, err
			}
			return []any{issuance}, nil
		}},
		{"native_getControlSwitches", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)
			charger.Charge(thor.SloadGas)

			switches, err := Params.Native(env.State()).Get(thor.KeyStakerSwitches)
			if err != nil {
				return nil, err
			}
			return []any{switches}, nil
		}},
	}
	stakerAbi := Staker.NativeABI()
	for _, def := range defines {
		if method, found := stakerAbi.MethodByName(def.name); found {
			nativeMethods[methodKey{Staker.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: func(env *xenv.Environment) []any {
					results, err := def.run(env)
					if err == nil {
						return results
					}
					if staker.IsRevertErr(err) {
						env.Revert(fmt.Sprintf("staker: %s", err.Error()))
						return nil
					}
					panic(err) // unexpected error
				},
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
