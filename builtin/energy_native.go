// Copyright (c) 2018 The VeChainThor developers

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
		run  func(env *xenv.Environment) []any
	}{
		{"native_totalSupply", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			env.ForkConfig()
			supply, err := env.Energy().TotalSupply()
			if err != nil {
				panic(err)
			}
			return []any{supply}
		}},
		{"native_totalBurned", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			burned, err := env.Energy().TotalBurned()
			if err != nil {
				panic(err)
			}
			return []any{burned}
		}},
		{"native_get", func(env *xenv.Environment) []any {
			var addr common.Address
			env.ParseArgs(&addr)

			env.UseGas(thor.GetBalanceGas)
			bal, err := env.Energy().Get(thor.Address(addr))
			if err != nil {
				panic(err)
			}
			return []any{bal}
		}},
		{"native_add", func(env *xenv.Environment) []any {
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
			if err := env.Energy().Add(thor.Address(args.Addr), args.Amount); err != nil {
				panic(err)
			}
			return nil
		}},
		{"native_sub", func(env *xenv.Environment) []any {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)
			if args.Amount.Sign() == 0 {
				return []any{true}
			}

			env.UseGas(thor.GetBalanceGas)
			ok, err := env.Energy().Sub(thor.Address(args.Addr), args.Amount)
			if err != nil {
				panic(err)
			}
			if ok {
				env.UseGas(thor.SstoreResetGas)
			}
			return []any{ok}
		}},
		{"native_master", func(env *xenv.Environment) []any {
			var addr common.Address
			env.ParseArgs(&addr)

			env.UseGas(thor.GetBalanceGas)
			master, err := env.State().GetMaster(thor.Address(addr))
			if err != nil {
				panic(err)
			}
			return []any{master}
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
