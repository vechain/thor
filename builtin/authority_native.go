// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/xenv"
)

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) []interface{}
	}{
		{"native_getExecutor", func(env *xenv.Environment) []interface{} {
			return []interface{}{Executor.Address}
		}},
		{"native_add", func(env *xenv.Environment) []interface{} {
			var args struct {
				Signer   common.Address
				Endorsor common.Address
				Identity common.Hash
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			ok := Authority.Native(env.State()).Add(thor.Address(args.Signer), thor.Address(args.Endorsor), thor.Bytes32(args.Identity))
			if ok {
				env.UseGas(thor.SstoreSetGas)
				env.UseGas(thor.SstoreResetGas)
			}
			return []interface{}{ok}
		}},
		{"native_remove", func(env *xenv.Environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)

			env.UseGas(thor.SloadGas)
			ok := Authority.Native(env.State()).Remove(thor.Address(signer))
			if ok {
				env.UseGas(thor.SstoreResetGas * 2)
			}
			return []interface{}{ok}
		}},
		{"native_get", func(env *xenv.Environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)

			env.UseGas(thor.SloadGas)
			p := Authority.Native(env.State()).Get(thor.Address(signer))
			return []interface{}{!p.IsEmpty(), p.Endorsor, p.Identity, p.Active}
		}},
		{"native_first", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			signer := Authority.Native(env.State()).First()
			return []interface{}{signer}
		}},
		{"native_next", func(env *xenv.Environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)

			env.UseGas(thor.SloadGas)
			p := Authority.Native(env.State()).Get(thor.Address(signer))
			var next thor.Address
			if p.Next != nil {
				next = *p.Next
			}
			return []interface{}{next}
		}},
		{"native_isEndorsed", func(env *xenv.Environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)

			env.UseGas(thor.SloadGas)
			p := Authority.Native(env.State()).Get(thor.Address(signer))
			if p.IsEmpty() {
				return []interface{}{false}
			}

			env.UseGas(thor.GetBalanceGas)
			bal := env.State().GetBalance(p.Endorsor)

			env.UseGas(thor.SloadGas)
			endorsement := Params.Native(env.State()).Get(thor.KeyProposerEndorsement)
			return []interface{}{bal.Cmp(endorsement) >= 0}
		}},
	}
	nativeABI := Authority.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[methodKey{Authority.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
