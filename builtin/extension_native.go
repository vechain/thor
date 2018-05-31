package builtin

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/xenv"
)

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) []interface{}
	}{
		{"native_blake2b256", func(env *xenv.Environment) []interface{} {
			var data []byte
			env.ParseArgs(&data)
			env.UseGas(uint64(len(data)+31)/32*blake2b256WordGas + blake2b256Gas)
			output := thor.Blake2b(data)
			return []interface{}{output}
		}},
		{"native_getBlockIDByNum", func(env *xenv.Environment) []interface{} {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			env.UseGas(thor.SloadGas)
			env.Require(blockNum >= 0 && blockNum < env.BlockContext().Number)
			output := env.Seeker().GetID(blockNum)
			return []interface{}{output}
		}},
		{"native_getTotalScoreByNum", func(env *xenv.Environment) []interface{} {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			env.UseGas(thor.SloadGas)
			env.Require(blockNum >= 0 && blockNum <= env.BlockContext().Number)
			if blockNum == env.BlockContext().Number {
				return []interface{}{env.BlockContext().TotalScore}
			}
			id := env.Seeker().GetID(blockNum)

			env.UseGas(thor.SloadGas)
			header := env.Seeker().GetHeader(id)
			return []interface{}{header.TotalScore()}
		}},
		{"native_getTimestampByNum", func(env *xenv.Environment) []interface{} {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			env.UseGas(thor.SloadGas)
			env.Require(blockNum >= 0 && blockNum <= env.BlockContext().Number)
			if blockNum == env.BlockContext().Number {
				return []interface{}{env.BlockContext().Time}
			}
			id := env.Seeker().GetID(blockNum)

			env.UseGas(thor.SloadGas)
			header := env.Seeker().GetHeader(id)
			return []interface{}{header.Timestamp()}
		}},
		{"native_getProposerByNum", func(env *xenv.Environment) []interface{} {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			env.UseGas(thor.SloadGas)
			env.Require(blockNum >= 0 && blockNum <= env.BlockContext().Number)
			if blockNum == env.BlockContext().Number {
				return []interface{}{env.BlockContext().Proposer}
			}

			id := env.Seeker().GetID(blockNum)

			env.UseGas(thor.SloadGas)
			header := env.Seeker().GetHeader(id)
			proposer, _ := header.Signer()
			return []interface{}{proposer}
		}},
		{"native_getTokenTotalSupply", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			output := Energy.Native(env.State()).GetTokenTotalSupply()
			return []interface{}{output}
		}},
		{"native_getTransactionProvedWork", func(env *xenv.Environment) []interface{} {
			env.UseGas(thor.SloadGas)
			output := env.TransactionContext().ProvedWork
			return []interface{}{output}
		}},
	}

	nativeABI := Extension.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[methodKey{Extension.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
