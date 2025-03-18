// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) []any
	}{
		{"native_executor", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)

			val, err := Params.Native(env.State()).Get(thor.KeyExecutorAddress)
			if err != nil {
				panic(err)
			}

			addr := thor.BytesToAddress(val.Bytes())
			return []any{addr}
		}},
		{"native_add", func(env *xenv.Environment) []any {
			var args struct {
				NodeMaster common.Address
				Endorsor   common.Address
				Identity   common.Hash
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			ok, err := Authority.Native(env.State()).Add(
				thor.Address(args.NodeMaster),
				thor.Address(args.Endorsor),
				thor.Bytes32(args.Identity))
			if err != nil {
				panic(err)
			}

			if ok {
				env.UseGas(thor.SstoreSetGas)
				env.UseGas(thor.SstoreResetGas)
			}
			return []any{ok}
		}},
		{"native_revoke", func(env *xenv.Environment) []any {
			var nodeMaster common.Address
			env.ParseArgs(&nodeMaster)

			env.UseGas(thor.SloadGas)
			ok, err := Authority.Native(env.State()).Revoke(thor.Address(nodeMaster))
			if err != nil {
				panic(err)
			}
			if ok {
				env.UseGas(thor.SstoreResetGas * 3)
			}
			return []any{ok}
		}},
		{"native_get", func(env *xenv.Environment) []any {
			var nodeMaster common.Address
			env.ParseArgs(&nodeMaster)

			env.UseGas(thor.SloadGas * 2)
			listed, endorsor, identity, active, err := Authority.Native(env.State()).Get(thor.Address(nodeMaster))
			if err != nil {
				panic(err)
			}

			return []any{listed, endorsor, identity, active}
		}},
		{"native_first", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			nodeMaster, err := Authority.Native(env.State()).First()
			if err != nil {
				panic(err)
			}
			if nodeMaster != nil {
				return []any{*nodeMaster}
			}
			return []any{thor.Address{}}
		}},
		{"native_next", func(env *xenv.Environment) []any {
			var nodeMaster common.Address
			env.ParseArgs(&nodeMaster)

			env.UseGas(thor.SloadGas)
			next, err := Authority.Native(env.State()).Next(thor.Address(nodeMaster))
			if err != nil {
				panic(err)
			}
			if next != nil {
				return []any{*next}
			}
			return []any{thor.Address{}}
		}},
		{"native_isEndorsed", func(env *xenv.Environment) []any {
			var nodeMaster common.Address
			env.ParseArgs(&nodeMaster)

			env.UseGas(thor.SloadGas * 2)
			listed, endorsor, _, _, err := Authority.Native(env.State()).Get(thor.Address(nodeMaster))
			if err != nil {
				panic(err)
			}
			if !listed {
				return []any{false}
			}

			env.UseGas(thor.GetBalanceGas)
			bal, err := env.State().GetBalance(endorsor)
			if err != nil {
				panic(err)
			}

			env.UseGas(thor.SloadGas)
			endorsement, err := Params.Native(env.State()).Get(thor.KeyProposerEndorsement)
			if err != nil {
				panic(err)
			}
			return []any{bal.Cmp(endorsement) >= 0}
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
