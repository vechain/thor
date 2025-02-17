// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) []interface{}
	}{
		{"native_totalStake", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			staked, err := Staker.Native(env.State(), env.BlockContext().Time).TotalStake()
			if err != nil {
				panic(err)
			}
			return []interface{}{staked}
		}},
		{"native_get", func(env *xenv.Environment) []interface{} {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)

			validator, err := Staker.Native(env.State(), env.BlockContext().Time).Get(thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			return []interface{}{validator.Stake, validator.Weight, validator.Status}
		}},
		{"native_firstActive", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			first, err := Staker.Native(env.State(), env.BlockContext().Time).FirstActive()
			if err != nil {
				panic(err)
			}
			return []interface{}{first}
		}},
		{"native_firstQueued", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			first, err := Staker.Native(env.State(), env.BlockContext().Time).FirstQueued()
			if err != nil {
				panic(err)
			}
			return []interface{}{first}
		}},
		{"native_next", func(env *xenv.Environment) []interface{} {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			next, err := Staker.Native(env.State(), env.BlockContext().Time).Next(thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			return []interface{}{next}
		}},
		{"native_withdraw", func(env *xenv.Environment) []interface{} {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)

			stake, err := Staker.Native(env.State(), env.BlockContext().Time).WithdrawStake(thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)

			return []interface{}{stake}
		}},
		{"native_addValidator", func(env *xenv.Environment) []interface{} {
			var args struct {
				Validator   common.Address
				Beneficiary common.Address
				Expiry      uint64
				Stake       *big.Int
			}
			env.ParseArgs(&args)

			err := Staker.Native(env.State(), env.BlockContext().Time).AddValidator(uint64(env.BlockContext().Number), thor.Address(args.Validator), thor.Address(args.Beneficiary), args.Expiry, args.Stake)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			return nil
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
