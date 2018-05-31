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
		{"native_getTotalSupply", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			supply := Energy.Native(env.State()).GetTotalSupply(env.BlockContext().Time)
			return []interface{}{supply}
		}},
		{"native_getTotalBurned", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			burned := Energy.Native(env.State()).GetTotalBurned()
			return []interface{}{burned}
		}},
		{"native_getBalance", func(env *xenv.Environment) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)

			env.UseGas(thor.GetBalanceGas)
			bal := Energy.Native(env.State()).GetBalance(thor.Address(addr), env.BlockContext().Time)
			return []interface{}{bal}
		}},
		{"native_addBalance", func(env *xenv.Environment) []interface{} {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)

			env.UseGas(thor.GetBalanceGas)
			if env.State().Exists(thor.Address(args.Addr)) {
				env.UseGas(thor.SstoreResetGas - thor.GetBalanceGas)
			} else {
				env.UseGas(thor.SstoreSetGas - thor.GetBalanceGas)
			}
			Energy.Native(env.State()).AddBalance(thor.Address(args.Addr), args.Amount, env.BlockContext().Time)
			return nil
		}},
		{"native_subBalance", func(env *xenv.Environment) []interface{} {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)

			env.UseGas(thor.GetBalanceGas)
			ok := Energy.Native(env.State()).SubBalance(thor.Address(args.Addr), args.Amount, env.BlockContext().Time)
			if ok {
				env.UseGas(thor.SstoreResetGas - thor.GetBalanceGas)
			}
			return []interface{}{ok}
		}},
	}
	abi := Energy.NativeABI()
	for _, def := range defines {
		if method, found := abi.MethodByName(def.name); found {
			privateMethods[methodKey{Energy.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
