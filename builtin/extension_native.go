// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

const (
	blake2b256WordGas uint64 = 3
	blake2b256Gas     uint64 = 15
)

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) []any
	}{
		{"native_blake2b256", func(env *xenv.Environment) []any {
			var data []byte
			env.ParseArgs(&data)
			env.UseGas(uint64(len(data)+31)/32*blake2b256WordGas + blake2b256Gas)
			output := thor.Blake2b(data)
			return []any{output}
		}},
		{"native_blockID", func(env *xenv.Environment) []any {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			if blockNum >= env.BlockContext().Number {
				return []any{thor.Bytes32{}}
			}

			env.UseGas(thor.SloadGas)
			output, err := env.Chain().GetBlockID(blockNum)
			if err != nil {
				panic(err)
			}
			return []any{output}
		}},
		{"native_blockTotalScore", func(env *xenv.Environment) []any {
			var blockNum uint32
			env.ParseArgs(&blockNum)

			if blockNum > env.BlockContext().Number {
				return []any{uint64(0)}
			}

			if blockNum == env.BlockContext().Number {
				return []any{env.BlockContext().TotalScore}
			}

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)
			header, err := env.Chain().GetBlockHeader(blockNum)
			if err != nil {
				panic(err)
			}
			return []any{header.TotalScore()}
		}},
		{"native_blockTime", func(env *xenv.Environment) []any {
			var blockNum uint32
			env.ParseArgs(&blockNum)

			if blockNum > env.BlockContext().Number {
				return []any{uint64(0)}
			}

			if blockNum == env.BlockContext().Number {
				return []any{env.BlockContext().Time}
			}

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)
			header, err := env.Chain().GetBlockHeader(blockNum)
			if err != nil {
				panic(err)
			}
			return []any{header.Timestamp()}
		}},
		{"native_blockSigner", func(env *xenv.Environment) []any {
			var blockNum uint32
			env.ParseArgs(&blockNum)

			if blockNum > env.BlockContext().Number {
				return []any{thor.Address{}}
			}

			if blockNum == env.BlockContext().Number {
				return []any{env.BlockContext().Signer}
			}

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)
			header, err := env.Chain().GetBlockHeader(blockNum)
			if err != nil {
				panic(err)
			}
			signer, err := header.Signer()
			if err != nil {
				panic(err)
			}
			return []any{signer}
		}},
		{"native_totalSupply", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			output, err := Energy.Native(env.State(), env.BlockContext().Time).TokenTotalSupply()
			if err != nil {
				panic(err)
			}
			return []any{output}
		}},
		{"native_txProvedWork", func(env *xenv.Environment) []any {
			output := env.TransactionContext().ProvedWork
			return []any{output}
		}},
		{"native_txID", func(env *xenv.Environment) []any {
			output := env.TransactionContext().ID
			return []any{output}
		}},

		{"native_txBlockRef", func(env *xenv.Environment) []any {
			output := env.TransactionContext().BlockRef
			return []any{output}
		}},
		{"native_txExpiration", func(env *xenv.Environment) []any {
			output := env.TransactionContext().Expiration
			return []any{output}
		}},
		{"native_txGasPayer", func(env *xenv.Environment) []any {
			output := env.TransactionContext().GasPayer
			return []any{output}
		}},
	}

	abi := Extension.V2.NativeABI()
	for _, def := range defines {
		if method, found := abi.MethodByName(def.name); found {
			nativeMethods[methodKey{Extension.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
