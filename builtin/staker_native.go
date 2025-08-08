// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/vechain/thor/v2/builtin/staker/validation"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

func isContractPaused(state *state.State, charger *gascharger.Charger, pauseBit int) (bool, error) {
	charger.Charge(thor.SloadGas)
	switches, err := Params.Native(state).Get(thor.KeyStargateSwitches)
	if err != nil {
		return false, err
	}
	return switches.Bit(pauseBit) == 1, nil
}

func IsStargatePaused(state *state.State, charger *gascharger.Charger) error {
	isPaused, err := isContractPaused(state, charger, 0)
	if err != nil {
		return err
	}
	if isPaused {
		return errors.New("stargate is paused")
	}
	return nil
}

// The staker pause switch at binary position 1. (binary: 1 [1] 0)
func IsStakerPaused(state *state.State, charger *gascharger.Charger) error {
	isPaused, err := isContractPaused(state, charger, 1)
	if err != nil {
		return err
	}
	if isPaused {
		return errors.New("staker is paused")
	}
	return nil
}

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
		{"native_getValidatorStake", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).Get(thor.Address(args.Validator))
			if err != nil {
				return []any{
					thor.Address{},
					big.NewInt(0),
					big.NewInt(0),
					fmt.Sprintf("revert: %v", err),
				}
			}
			if validator.IsEmpty() {
				return []any{thor.Address{}, big.NewInt(0), big.NewInt(0), ""}
			}
			return []any{
				validator.Endorsor,
				validator.LockedVET,
				validator.Weight,
				validator.QueuedVET,
				"",
			}
		}},
		{"native_getValidatorStatus", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).Get(thor.Address(args.Validator))
			if err != nil {
				return []any{
					validation.StatusUnknown,
					false,
					fmt.Sprintf("revert: %v", err),
				}
			}
			if validator.IsEmpty() {
				return []any{validation.StatusUnknown, false, ""}
			}
			return []any{
				validator.Status,
				validator.Online,
				"",
			}
		}},
		{"native_getValidatorPeriodDetails", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).Get(thor.Address(args.Validator))
			if err != nil {
				return []any{
					uint32(0),
					uint32(0),
					uint32(0),
					uint32(0),
					fmt.Sprintf("revert: %v", err),
				}
			}
			if validator.IsEmpty() {
				return []any{uint32(0), uint32(0), uint32(0), uint32(0), ""}
			}
			exitBlock := uint32(math.MaxUint32)
			if validator.ExitBlock != nil {
				exitBlock = *validator.ExitBlock
			}
			return []any{
				validator.Period,
				validator.StartBlock,
				exitBlock,
				validator.CompleteIterations,
				"",
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
				Validator common.Address
				Endorsor  common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawStake(
				thor.Address(args.Validator),
				thor.Address(args.Endorsor),
				env.BlockContext().Number,
			)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			return []any{stake, ""}
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

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			isPoSActive, err := Staker.NativeMetered(env.State(), charger).IsPoSActive()
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			if !isPoSActive {
				charger.Charge(thor.SloadGas) // a.getEntry(ValidatorMaster)

				exists, endorsor, _, _, err := Authority.Native(env.State()).Get(thor.Address(args.Validator))
				if err != nil {
					return []any{fmt.Sprintf("revert: %v", err)}
				}
				if !exists {
					return []any{"revert: Validation is not present in the Authority"}
				}
				if thor.Address(args.Endorsor) != endorsor {
					return []any{"revert: endorsor is not present in the Authority"}
				}
			}

			err = Staker.NativeMetered(env.State(), charger).
				AddValidation(
					thor.Address(args.Validator),
					thor.Address(args.Endorsor),
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
				Validator common.Address
				Endorsor  common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			err = Staker.NativeMetered(env.State(), charger).
				SignalExit(
					thor.Address(args.Validator),
					thor.Address(args.Endorsor),
				)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}
			return []any{""}
		}},
		{"native_increaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
				Endorsor  common.Address
				Amount    *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			err = Staker.NativeMetered(env.State(), charger).
				IncreaseStake(
					thor.Address(args.Validator),
					thor.Address(args.Endorsor),
					args.Amount,
				)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			return []any{""}
		}},
		{"native_decreaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
				Endorsor  common.Address
				Amount    *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			err = Staker.NativeMetered(env.State(), charger).
				DecreaseStake(
					thor.Address(args.Validator),
					thor.Address(args.Endorsor),
					args.Amount,
				)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}
			return []any{""}
		}},
		{"native_addDelegation", func(env *xenv.Environment) []any {
			var args struct {
				Validator  common.Address
				Stake      *big.Int
				Multiplier uint8
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStargatePaused(env.State(), charger)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			err = IsStakerPaused(env.State(), charger)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			delegationID, err := Staker.NativeMetered(env.State(), charger).
				AddDelegation(
					thor.Address(args.Validator),
					args.Stake,
					args.Multiplier,
				)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{delegationID, ""}
		}},
		{"native_withdrawDelegation", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStargatePaused(env.State(), charger)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			err = IsStakerPaused(env.State(), charger)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawDelegation(args.DelegationID)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			return []any{stake, ""}
		}},
		{"native_signalDelegationExit", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStargatePaused(env.State(), charger)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			err = IsStakerPaused(env.State(), charger)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			err = Staker.NativeMetered(env.State(), charger).SignalDelegationExit(args.DelegationID)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}

			return []any{""}
		}},
		{"native_getDelegationStake", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegation, _, err := Staker.NativeMetered(env.State(), charger).GetDelegation(args.DelegationID)
			if err != nil {
				return []any{thor.Address{}, new(big.Int), uint8(0), fmt.Sprintf("revert: %v", err)}
			}

			return []any{
				delegation.Validation,
				delegation.Stake,
				delegation.Multiplier,
				"",
			}
		}},
		{"native_getDelegationPeriodDetails", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegation, validator, err := Staker.NativeMetered(env.State(), charger).GetDelegation(args.DelegationID)
			if err != nil {
				return []any{uint32(0), uint32(0), false, fmt.Sprintf("revert: %v", err)}
			}

			lastPeriod := uint32(math.MaxUint32)
			if delegation.LastIteration != nil {
				lastPeriod = *delegation.LastIteration
			}

			locked := delegation.Started(validator) && !delegation.Ended(validator)

			return []any{
				delegation.FirstIteration,
				lastPeriod,
				locked,
				"",
			}
		}},
		{"native_getDelegatorsRewards", func(env *xenv.Environment) []any {
			var args struct {
				Validator     common.Address
				StakingPeriod uint32
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			reward, err := Staker.NativeMetered(env.State(), charger).GetDelegatorRewards(thor.Address(args.Validator), args.StakingPeriod)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{reward, ""}
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
		{"native_getValidationTotals", func(env *xenv.Environment) []any {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			totals, err := Staker.NativeMetered(env.State(), charger).GetValidationTotals(thor.Address(args.Validator))
			if err != nil {
				return []any{
					new(big.Int),
					new(big.Int),
					new(big.Int),
					new(big.Int),
					new(big.Int),
					new(big.Int),
					fmt.Sprintf("revert: failed to get validators totals: %v", err),
				}
			}
			return []any{
				totals.TotalLockedStake,
				totals.TotalLockedWeight,
				totals.TotalQueuedStake,
				totals.TotalQueuedWeight,
				totals.TotalExitingStake,
				totals.TotalExitingWeight,
				"",
			}
		}},
		{"native_getValidatorsNum", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			leaderGroupSize, queuedGroupSize, err := Staker.NativeMetered(env.State(), charger).GetValidatorsNum()
			if err != nil {
				return []any{new(big.Int), new(big.Int), fmt.Sprintf("revert: failed to get validators totals: %v", err)}
			}
			return []any{leaderGroupSize, queuedGroupSize, ""}
		}},
		{"native_issuance", func(env *xenv.Environment) []any {
			charger := gascharger.New(env)

			staker := Staker.NativeMetered(env.State(), charger)
			issuance, err := Energy.Native(env.State(), env.BlockContext().Time).CalculateRewards(staker)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{issuance, ""}
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
