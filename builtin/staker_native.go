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
			staked, err := Staker.Native(env.State()).TotalStake()
			if err != nil {
				panic(err)
			}
			return []any{staked}
		}},
		{"native_activeStake", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			staked, err := Staker.Native(env.State()).ActiveStake()
			if err != nil {
				panic(err)
			}
			return []any{staked}
		}},
		{"native_get", func(env *xenv.Environment) []any {
			var args struct {
				Master common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)

			validator, err := Staker.Native(env.State()).Get(thor.Address(args.Master))
			if err != nil {
				panic(err)
			}
			if validator.IsEmpty() {
				return []any{thor.Address{}, big.NewInt(0), big.NewInt(0), staker.StatusUnknown}
			}
			return []any{validator.Endorsor, validator.Stake, validator.Weight, validator.Status}
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
				Prev common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			next, err := Staker.Native(env.State()).Next(thor.Address(args.Prev))
			if err != nil {
				panic(err)
			}
			return []any{next}
		}},
		{"native_withdraw", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor common.Address
				Master   common.Address
			}
			env.ParseArgs(&args)

			stake, err := Staker.Native(env.State()).WithdrawStake(thor.Address(args.Endorsor), thor.Address(args.Master))
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

			err := Staker.Native(env.State()).AddValidator(thor.Address(args.Endorsor), thor.Address(args.Master), args.Period, args.Stake, args.AutoRenew)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			return nil
		}},

		{"native_updateAutoRenew", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor  common.Address
				Master    common.Address
				AutoRenew bool
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State()).UpdateAutoRenew(thor.Address(args.Endorsor), thor.Address(args.Master), args.AutoRenew, env.BlockContext().Number)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			return nil
		}},
		{"native_increaseStake", func(env *xenv.Environment) []any {
			var args struct {
				Endorsor common.Address
				Master   common.Address
				Stake    *big.Int
			}
			env.ParseArgs(&args)

			newStake, err := Staker.Native(env.State()).IncreaseStake(thor.Address(args.Endorsor), thor.Address(args.Master), args.Stake)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			return []any{newStake}
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
