// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/xenv"
)

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) []interface{}
	}{
		{"native_totalSupply", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			supply, err := Energy.Native(env.State(), env.BlockContext().Time).TotalSupply()
			if err != nil {
				panic(err)
			}
			return []interface{}{supply}
		}},
		{"native_totalBurned", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			burned, err := Energy.Native(env.State(), env.BlockContext().Time).TotalBurned()
			if err != nil {
				panic(err)
			}
			return []interface{}{burned}
		}},
		{"native_get", func(env *xenv.Environment) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)

			env.UseGas(thor.GetBalanceGas)
			bal, err := Energy.Native(env.State(), env.BlockContext().Time).Get(thor.Address(addr))
			if err != nil {
				panic(err)
			}
			return []interface{}{bal}
		}},
		{"native_add", func(env *xenv.Environment) []interface{} {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)
			if args.Amount.Sign() == 0 {
				return nil
			}

			env.UseGas(thor.GetBalanceGas)

			exist, err := env.State().Exists(thor.Address(args.Addr))
			if err != nil {
				panic(err)
			}
			if exist {
				env.UseGas(thor.SstoreResetGas)
			} else {
				env.UseGas(thor.SstoreSetGas)
			}
			if err := Energy.Native(env.State(), env.BlockContext().Time).Add(thor.Address(args.Addr), args.Amount); err != nil {
				panic(err)
			}
			return nil
		}},
		{"native_sub", func(env *xenv.Environment) []interface{} {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)
			if args.Amount.Sign() == 0 {
				return []interface{}{true}
			}

			env.UseGas(thor.GetBalanceGas)
			ok, err := Energy.Native(env.State(), env.BlockContext().Time).Sub(thor.Address(args.Addr), args.Amount)
			if err != nil {
				panic(err)
			}
			if ok {
				env.UseGas(thor.SstoreResetGas)
			}
			return []interface{}{ok}
		}},
		{"native_master", func(env *xenv.Environment) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)

			env.UseGas(thor.GetBalanceGas)
			master, err := env.State().GetMaster(thor.Address(addr))
			if err != nil {
				panic(err)
			}
			return []interface{}{master}
		}},
	}
	abi := Energy.NativeABI()
	for _, def := range defines {
		if method, found := abi.MethodByName(def.name); found {
			nativeMethods[methodKey{Energy.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
