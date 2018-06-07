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
		{"native_executor", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			addr := thor.BytesToAddress(Params.Native(env.State()).Get(thor.KeyExecutorAddress).Bytes())
			return []interface{}{addr}
		}},
		{"native_get", func(env *xenv.Environment) []interface{} {
			var key common.Hash
			env.ParseArgs(&key)

			env.UseGas(thor.SloadGas)
			v := Params.Native(env.State()).Get(thor.Bytes32(key))
			return []interface{}{v}
		}},
		{"native_set", func(env *xenv.Environment) []interface{} {
			var args struct {
				Key   common.Hash
				Value *big.Int
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SstoreSetGas)
			Params.Native(env.State()).Set(thor.Bytes32(args.Key), args.Value)
			return nil
		}},
	}
	abi := Params.NativeABI()
	for _, def := range defines {
		if method, found := abi.MethodByName(def.name); found {
			nativeMethods[methodKey{Params.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
