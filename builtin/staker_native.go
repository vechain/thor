// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
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
			staked, err := Staker.Native(env.State()).LockedVET()
			if err != nil {
				panic(err)
			}
			return []any{staked}
		}},
		{"native_queuedStake", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			staked, err := Staker.Native(env.State()).QueuedStake()
			if err != nil {
				panic(err)
			}
			return []any{staked}
		}},
		{"native_get", func(env *xenv.Environment) []any {
			var args struct {
				Id common.Hash // nolint: revive
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)

			validator, err := Staker.Native(env.State()).Get(thor.Bytes32(args.Id))
			if err != nil {
				panic(err)
			}
			if validator.IsEmpty() {
				return []any{thor.Address{}, thor.Address{}, big.NewInt(0), big.NewInt(0), staker.StatusUnknown, false}
			}
			return []any{validator.Master, validator.Endorsor, validator.LockedVET, validator.Weight, validator.Status, validator.AutoRenew}
		}},
		{"native_getWithdraw", func(env *xenv.Environment) []any {
			var args struct {
				Id common.Hash // nolint: revive
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)

			validator, err := Staker.Native(env.State()).Get(thor.Bytes32(args.Id))
			if err != nil {
				panic(err)
			}
			if validator.IsEmpty() {
				return []any{big.NewInt(0)}
			}
			return []any{validator.WithdrawableVET}
		}},
		{"native_firstActive", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			first, err := Staker.Native(env.State()).FirstActive()
			if err != nil {
				panic(err)
			}
			return []any{first}
		}},
		{"native_firstQueued", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			first, err := Staker.Native(env.State()).FirstQueued()
			if err != nil {
				panic(err)
			}
			return []any{first}
		}},
		{"native_next", func(env *xenv.Environment) []any {
			var args struct {
				Prev common.Hash
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			next, err := Staker.Native(env.State()).Next(thor.Bytes32(args.Prev))
			if err != nil {
				panic(err)
			}
			return []any{next}
		}},
		{"native_withdraw", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor common.Address
				Id       common.Hash // nolint: revive
			}
			env.ParseArgs(&args)

			stake, err := Staker.Native(env.State()).WithdrawStake(thor.Address(args.Endorsor), thor.Bytes32(args.Id))
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)

			return []any{stake}
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

			id, err := Staker.Native(env.State()).AddValidator(thor.Address(args.Endorsor), thor.Address(args.Master), args.Period, args.Stake, args.AutoRenew, env.BlockContext().Number)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			return []any{id}
		}},

		{"native_updateAutoRenew", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor  common.Address
				Id        common.Hash // nolint: revive
				AutoRenew bool
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).UpdateAutoRenew(thor.Address(args.Endorsor), thor.Bytes32(args.Id), args.AutoRenew, env.BlockContext().Number)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			return nil
		}},
		{"native_increaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor common.Address
				Id       common.Hash // nolint: revive
				Stake    *big.Int
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).IncreaseStake(thor.Address(args.Endorsor), thor.Bytes32(args.Id), args.Stake)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			return nil
		}},
		{"native_decreaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor common.Address
				Id       common.Hash // nolint: revive
				Stake    *big.Int
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).DecreaseStake(thor.Address(args.Endorsor), thor.Bytes32(args.Id), args.Stake)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			return nil
		}},
		{"native_addDelegation", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
				Delegator    common.Address
				Stake        *big.Int
				AutoRenew    bool
				Multiplier   uint8
			}
			env.ParseArgs(&args)
			err := Staker.Native(env.State()).AddDelegator(thor.Bytes32(args.ValidationID), thor.Address(args.Delegator), args.Stake, args.AutoRenew, args.Multiplier)
			if err != nil {
				panic(err)
			}
			return nil
		}},
		{"native_withdrawDelegation", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
				Delegator    common.Address
			}
			env.ParseArgs(&args)

			stake, err := Staker.Native(env.State()).DelegatorWithdrawStake(thor.Bytes32(args.ValidationID), thor.Address(args.Delegator))
			if err != nil {
				panic(err)
			}

			return []any{stake}
		}},
		{"native_updateDelegatorAutoRenew", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
				Delegator    common.Address
				AutoRenew    bool
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).UpdateDelegatorAutoRenew(thor.Bytes32(args.ValidationID), thor.Address(args.Delegator), args.AutoRenew)
			if err != nil {
				panic(err)
			}

			return nil
		}},
		{"native_getDelegation", func(env *xenv.Environment) []any {
			var args struct {
				ValidationID common.Hash
				Delegator    common.Address
			}

			env.ParseArgs(&args)

			delegation, err := Staker.Native(env.State()).GetDelegator(thor.Bytes32(args.ValidationID), thor.Address(args.Delegator))
			if err != nil {
				panic(err)
			}

			return []any{delegation.Stake, delegation.Multiplier, delegation.AutoRenew}
		}},
		{"native_getDelegatorContract", func(env *xenv.Environment) []any {
			// TODO: This is a quick hack for testing. Any address that calls the staker can be the delegator contract
			return []any{env.TransactionContext().Origin}
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
