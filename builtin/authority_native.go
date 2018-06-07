// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/builtin/authority"
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
		{"native_add", func(env *xenv.Environment) []interface{} {
			var args struct {
				Signer   common.Address
				Endorsor common.Address
				Identity common.Hash
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			ok := Authority.Native(env.State()).Add(&authority.Candidate{
				Signer:   thor.Address(args.Signer),
				Endorsor: thor.Address(args.Endorsor),
				Identity: thor.Bytes32(args.Identity),
				Active:   true, // set to active by default
			})
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
				env.UseGas(thor.SstoreResetGas * 3)
			}
			return []interface{}{ok}
		}},
		{"native_get", func(env *xenv.Environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)

			env.UseGas(thor.SloadGas)
			if candidate, ok := Authority.Native(env.State()).Get(thor.Address(signer)); ok {
				return []interface{}{true, candidate.Endorsor, candidate.Identity, candidate.Active}
			}
			return []interface{}{false, thor.Address{}, thor.Bytes32{}, false}
		}},
		{"native_first", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			if signer := Authority.Native(env.State()).First(); signer != nil {
				return []interface{}{*signer}
			}
			return []interface{}{thor.Address{}}
		}},
		{"native_next", func(env *xenv.Environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)

			env.UseGas(thor.SloadGas)
			if next := Authority.Native(env.State()).Next(thor.Address(signer)); next != nil {
				return []interface{}{*next}
			}
			return []interface{}{thor.Address{}}
		}},
		{"native_isEndorsed", func(env *xenv.Environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)

			env.UseGas(thor.SloadGas)
			if candidate, ok := Authority.Native(env.State()).Get(thor.Address(signer)); ok {
				env.UseGas(thor.GetBalanceGas)
				bal := env.State().GetBalance(candidate.Endorsor)

				env.UseGas(thor.SloadGas)
				endorsement := Params.Native(env.State()).Get(thor.KeyProposerEndorsement)
				return []interface{}{bal.Cmp(endorsement) >= 0}
			}
			return []interface{}{false}
		}},
	}
	abi := Authority.NativeABI()
	for _, def := range defines {
		if method, found := abi.MethodByName(def.name); found {
			nativeMethods[methodKey{Authority.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
