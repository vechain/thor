// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
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
			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			staked, weight, err := Staker.Native(env.State()).LockedVET()
			if err != nil {
				return []any{new(big.Int), new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{staked, weight, ""}
		}},
		{"native_queuedStake", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			staked, weight, err := Staker.Native(env.State()).QueuedStake()
			if err != nil {
				return []any{new(big.Int), new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{staked, weight, ""}
		}},
		{"native_get", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)

			validator, err := Staker.Native(env.State()).Get(thor.Bytes32(args.ValidationID))
			if err != nil {
				return []any{thor.Address{}, thor.Address{}, big.NewInt(0), big.NewInt(0), staker.StatusUnknown, false, false, uint32(0), fmt.Sprintf("revert: %v", err)}
			}
			if validator.IsEmpty() {
				return []any{thor.Address{}, thor.Address{}, big.NewInt(0), big.NewInt(0), staker.StatusUnknown, false, false, uint32(0), ""}
			}
			return []any{validator.Master, validator.Endorsor, validator.LockedVET, validator.Weight, validator.Status, validator.AutoRenew, validator.Online, validator.Period, ""}
		}},
		{"native_getWithdraw", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)

			amount, err := Staker.Native(env.State()).GetWithdrawable(thor.Bytes32(args.ValidationID), env.BlockContext().Number)
			if err != nil {
				return []any{big.NewInt(0), fmt.Sprintf("revert: %v", err)}
			}
			return []any{amount, ""}
		}},
		{"native_firstActive", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			first, err := Staker.Native(env.State()).FirstActive()
			if err != nil {
				return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
			}
			return []any{first, ""}
		}},
		{"native_firstQueued", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			first, err := Staker.Native(env.State()).FirstQueued()
			if err != nil {
				return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
			}
			return []any{first, ""}
		}},
		{"native_next", func(env *xenv.Environment) []any {
			var args struct {
				Prev common.Hash
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			next, err := Staker.Native(env.State()).Next(thor.Bytes32(args.Prev))
			if err != nil {
				return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
			}
			return []any{next, ""}
		}},
		{"native_withdraw", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor     common.Address
				ValidationID common.Hash
			}
			env.ParseArgs(&args)

			stake, err := Staker.Native(env.State()).WithdrawStake(thor.Address(args.Endorsor), thor.Bytes32(args.ValidationID), env.BlockContext().Number)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			env.UseGas(thor.SstoreSetGas)

			return []any{stake, ""}
		}},
		{"native_addValidator", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor  common.Address
				Master    common.Address
				Period    uint32
				Stake     *big.Int
				AutoRenew bool
			}
			env.ParseArgs(&args)

			isActive, err := Staker.Native(env.State()).IsActive()
			if err != nil {
				return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
			}

			if !isActive {
				exists, endorsor, _, _, err := Authority.Native(env.State()).Get(thor.Address(args.Master))
				if err != nil {
					return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
				}
				if !exists {
					return []any{thor.Bytes32{}, "revert: master is not present in the Authority"}
				}
				if thor.Address(args.Endorsor) != endorsor {
					return []any{thor.Bytes32{}, "revert: endorsor is not present in the Authority"}
				}
			}

			id, err := Staker.Native(env.State()).AddValidator(thor.Address(args.Endorsor), thor.Address(args.Master), args.Period, args.Stake, args.AutoRenew, env.BlockContext().Number)
			if err != nil {
				return []any{thor.Bytes32{}, fmt.Sprintf("revert: %v", err)}
			}
			env.UseGas(thor.SstoreSetGas)
			return []any{id, ""}
		}},

		{"native_updateAutoRenew", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor     common.Address
				ValidationID common.Hash
				AutoRenew    bool
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).UpdateAutoRenew(thor.Address(args.Endorsor), thor.Bytes32(args.ValidationID), args.AutoRenew)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}
			env.UseGas(thor.SstoreSetGas)
			return []any{""}
		}},
		{"native_increaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor     common.Address
				ValidationID common.Hash
				Amount       *big.Int
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).IncreaseStake(thor.Address(args.Endorsor), thor.Bytes32(args.ValidationID), args.Amount)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}
			env.UseGas(thor.SstoreSetGas)
			return []any{""}
		}},
		{"native_decreaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor     common.Address
				ValidationID common.Hash
				Amount       *big.Int
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).DecreaseStake(thor.Address(args.Endorsor), thor.Bytes32(args.ValidationID), args.Amount)
			if err != nil {
				return []any{fmt.Sprintf("revert: %v", err)}
			}
			env.UseGas(thor.SstoreSetGas)
			return []any{""}
		}},
		{"native_addDelegation", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
				Stake        *big.Int
				AutoRenew    bool
				Multiplier   uint8
			}
			env.ParseArgs(&args)
			delegationID, err := Staker.Native(env.State()).AddDelegation(thor.Bytes32(args.ValidationID), args.Stake, args.AutoRenew, args.Multiplier)
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

			stake, err := Staker.Native(env.State()).WithdrawDelegation(thor.Bytes32(args.DelegationID))
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}

			return []any{stake, ""}
		}},
		{"native_updateDelegationAutoRenew", func(env *xenv.Environment) []any {
			var args struct {
				DelegationID common.Hash
				AutoRenew    bool
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).UpdateDelegationAutoRenew(thor.Bytes32(args.DelegationID), args.AutoRenew)
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
			delegation, validator, err := Staker.Native(env.State()).GetDelegation(thor.Bytes32(args.DelegationID))
			if err != nil {
				return []any{thor.Bytes32{}, new(big.Int), uint32(0), uint32(0), uint8(0), false, false, fmt.Sprintf("revert: %v", err)}
			}
			var lastPeriod uint32 = math.MaxUint32
			if delegation.LastIteration != nil {
				lastPeriod = *delegation.LastIteration
			}
			return []any{delegation.ValidatorID, delegation.Stake, delegation.FirstIteration, lastPeriod, delegation.Multiplier, delegation.AutoRenew, delegation.IsLocked(validator), ""}
		}},
		{"native_getRewards", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID  common.Hash
				StakingPeriod uint32
			}
			env.ParseArgs(&args)
			reward, err := Staker.Native(env.State()).GetRewards(thor.Bytes32(args.ValidationID), args.StakingPeriod)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{reward, ""}
		}},
		{"native_getCompletedPeriods", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
			}
			env.ParseArgs(&args)
			periods, err := Staker.Native(env.State()).GetCompletedPeriods(thor.Bytes32(args.ValidationID))
			if err != nil {
				return []any{uint32(0), fmt.Sprintf("revert: %v", err)}
			}
			return []any{periods, ""}
		}},
		{"native_getDelegatorContract", func(env *xenv.Environment) []any {
			raw, err := Params.Native(env.State()).Get(thor.KeyStargateContractAddress)
			if err != nil {
				return []any{thor.Address{}, fmt.Sprintf("failed to get Stargate contract address: %v", err)}
			}
			addr := thor.BytesToAddress(raw.Bytes())
			return []any{addr, ""}
		}},
		{"native_getValidatorTotals", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
			}
			env.ParseArgs(&args)
			totals, err := Staker.Native(env.State()).GetValidatorsTotals(thor.Bytes32(args.ValidationID))
			if err != nil {
				return []any{new(big.Int), new(big.Int), new(big.Int), new(big.Int), fmt.Sprintf("failed to validators totals: %v", err)}
			}
			return []any{totals.TotalLockedStake, totals.TotalLockedWeight, totals.DelegationsLockedStake, totals.DelegationsLockedWeight, ""}
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
