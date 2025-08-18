// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/staker/reverts"
	"github.com/vechain/thor/v2/builtin/staker/validation"
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
		return reverts.New("stargate is paused")
	}
	return nil
}

// IsStakerPaused The staker pause switch at binary position 1. (binary: 1 [1] 0)
func IsStakerPaused(state *state.State, charger *gascharger.Charger) error {
	isPaused, err := isContractPaused(state, charger, 1)
	if err != nil {
		return err
	}
	if isPaused {
		return reverts.New("staker is paused")
	}
	return nil
}

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) ([]any, error)
	}{
		{"native_totalStake", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			staked, weight, err := Staker.NativeMetered(env.State(), charger).LockedVET()
			if err != nil {
				return nil, err
			}
			return []any{staked, weight}, nil
		}},
		{"native_queuedStake", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			staked, weight, err := Staker.NativeMetered(env.State(), charger).QueuedStake()
			if err != nil {
				return nil, err
			}
			return []any{staked, weight}, nil
		}},
		{"native_getValidatorStake", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).Get(thor.Address(args.Validator))
			if err != nil {
				return nil, err
			}
			if validator.IsEmpty() {
				return []any{thor.Address{}, big.NewInt(0), big.NewInt(0)}, nil
			}
			return []any{
				validator.Endorser,
				validator.LockedVET,
				validator.Weight,
				validator.QueuedVET,
			}, nil
		}},
		{"native_getValidatorStatus", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).Get(thor.Address(args.Validator))
			if err != nil {
				return nil, err
			}
			if validator.IsEmpty() {
				return []any{validation.StatusUnknown, false}, nil
			}
			return []any{
				validator.Status,
				validator.OfflineBlock == nil,
			}, nil
		}},
		{"native_getValidatorPeriodDetails", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			validator, err := Staker.NativeMetered(env.State(), charger).Get(thor.Address(args.Validator))
			if err != nil {
				return nil, err
			}
			if validator.IsEmpty() {
				return []any{uint32(0), uint32(0), uint32(0), uint32(0)}, nil
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
			return []any{amount}, nil
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

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawStake(
				thor.Address(args.Validator),
				thor.Address(args.Endorser),
				env.BlockContext().Number,
			)
			if err != nil {
				return nil, err
			}

			return []any{stake}, nil
		}},
		{"native_addValidation", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				Validator common.Address
				Endorser  common.Address
				Period    uint32
				Stake     *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			isPoSActive, err := Staker.NativeMetered(env.State(), charger).IsPoSActive()
			if err != nil {
				return nil, err
			}

			if !isPoSActive {
				charger.Charge(thor.SloadGas) // a.getEntry(ValidatorMaster)

				exists, endorser, _, _, err := Authority.Native(env.State()).Get(thor.Address(args.Validator))
				if err != nil {
					return nil, err
				}
				if !exists {
					return nil, reverts.New("validator is not registered in the Authority")
				}
				if thor.Address(args.Endorser) != endorser {
					return nil, reverts.New("endorser is not present in the Authority")
				}
			}

			err = Staker.NativeMetered(env.State(), charger).
				AddValidation(
					thor.Address(args.Validator),
					thor.Address(args.Endorser),
					args.Period,
					args.Stake,
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

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			err = Staker.NativeMetered(env.State(), charger).
				SignalExit(
					thor.Address(args.Validator),
					thor.Address(args.Endorser),
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

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			err = Staker.NativeMetered(env.State(), charger).
				IncreaseStake(
					thor.Address(args.Validator),
					thor.Address(args.Endorser),
					args.Amount,
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

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			err = Staker.NativeMetered(env.State(), charger).
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

			err := IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			err = Staker.NativeMetered(env.State(), charger).
				DecreaseStake(
					thor.Address(args.Validator),
					thor.Address(args.Endorser),
					args.Amount,
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

			err := IsStargatePaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			err = IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			delegationID, err := Staker.NativeMetered(env.State(), charger).
				AddDelegation(
					thor.Address(args.Validator),
					args.Stake,
					args.Multiplier,
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

			err := IsStargatePaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			err = IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			stake, err := Staker.NativeMetered(env.State(), charger).WithdrawDelegation(args.DelegationID)
			if err != nil {
				return nil, err
			}

			return []any{stake}, nil
		}},
		{"native_signalDelegationExit", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			err := IsStargatePaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			err = IsStakerPaused(env.State(), charger)
			if err != nil {
				return nil, err
			}

			err = Staker.NativeMetered(env.State(), charger).SignalDelegationExit(args.DelegationID)
			if err != nil {
				return nil, err
			}

			return []any{}, nil
		}},
		{"native_getDelegationStake", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegation, _, err := Staker.NativeMetered(env.State(), charger).GetDelegation(args.DelegationID)
			if err != nil {
				return nil, err
			}

			return []any{
				delegation.Validation,
				delegation.Stake,
				delegation.Multiplier,
			}, nil
		}},
		{"native_getDelegationPeriodDetails", func(env *xenv.Environment) ([]any, error) {
			var args struct {
				DelegationID *big.Int
			}
			env.ParseArgs(&args)
			charger := gascharger.New(env)

			delegation, validator, err := Staker.NativeMetered(env.State(), charger).GetDelegation(args.DelegationID)
			if err != nil {
				return nil, err
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
			raw, err := Params.Native(env.State()).Get(thor.KeyStargateContractAddress)
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
			return []any{
				totals.TotalLockedStake,
				totals.TotalLockedWeight,
				totals.TotalQueuedStake,
				totals.TotalQueuedWeight,
				totals.TotalExitingStake,
				totals.TotalExitingWeight,
			}, nil
		}},
		{"native_getValidatorsNum", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			leaderGroupSize, queuedGroupSize, err := Staker.NativeMetered(env.State(), charger).GetValidatorsNum()
			if err != nil {
				return nil, err
			}
			return []any{leaderGroupSize, queuedGroupSize}, nil
		}},
		{"native_issuance", func(env *xenv.Environment) ([]any, error) {
			charger := gascharger.New(env)

			staker := Staker.NativeMetered(env.State(), charger)
			issuance, err := Energy.Native(env.State(), env.BlockContext().Time).CalculateRewards(staker)
			if err != nil {
				return nil, err
			}
			return []any{issuance}, nil
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
					if reverts.IsRevertErr(err) {
						env.Revert(err.Error())
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
