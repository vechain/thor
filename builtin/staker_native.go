// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
	"math/big"
)

func init() {
	events := Staker.Events()

	mustEventByName := func(name string) *abi.Event {
		if event, found := events.EventByName(name); found {
			return event
		}
		panic("event not found")
	}

	stakedEvent := mustEventByName("Staked")
	unstakedEvent := mustEventByName("Unstaked")
	validatorAddedEvent := mustEventByName("ValidatorAdded")
	validatorRemovedEvent := mustEventByName("ValidatorRemoved")

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
		{"native_getStake", func(env *xenv.Environment) []interface{} {
			var args struct {
				Staker    common.Address
				Validator common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)

			staked, err := Staker.Native(env.State(), env.BlockContext().Time).GetStake(thor.Address(args.Staker), thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			return []interface{}{staked}
		}},
		{"native_stake", func(env *xenv.Environment) []interface{} {
			var args struct {
				Staker    common.Address
				Amount    *big.Int
				Validator common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			err := Staker.Native(env.State(), env.BlockContext().Time).Stake(thor.Address(args.Staker), thor.Address(args.Validator), args.Amount)
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			env.UseGas(thor.SstoreResetGas)
			env.Log(stakedEvent, env.To(), []thor.Bytes32{thor.BytesToBytes32(args.Staker[:]), thor.BytesToBytes32(args.Validator[:])}, args.Amount)
			return nil
		}},
		{"native_unstake", func(env *xenv.Environment) []interface{} {
			var args struct {
				Staker    common.Address
				Amount    *big.Int
				Validator common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			env.UseGas(thor.GetBalanceGas)
			err := Staker.Native(env.State(), env.BlockContext().Time).Unstake(thor.Address(args.Staker), args.Amount, thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			env.UseGas(thor.SstoreResetGas)
			env.Log(unstakedEvent, env.To(), []thor.Bytes32{thor.BytesToBytes32(args.Staker[:]), thor.BytesToBytes32(args.Validator[:])}, args.Amount)
			return nil
		}},
		{"native_listValidators", func(env *xenv.Environment) []interface{} {

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SstoreResetGas)
			validators, err := Staker.Native(env.State(), env.BlockContext().Time).ListValidators()
			if err != nil {
				panic(err)
			}
			return []interface{}{validators}
		}},
		{"native_addValidator", func(env *xenv.Environment) []interface{} {
			var args struct {
				Amount    *big.Int
				Validator common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			err := Staker.Native(env.State(), env.BlockContext().Time).AddValidator(args.Amount, thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			env.UseGas(thor.SstoreResetGas)
			env.Log(validatorAddedEvent, env.To(), []thor.Bytes32{thor.BytesToBytes32(args.Validator[:])})
			return nil
		}},
		{"native_removeValidator", func(env *xenv.Environment) []interface{} {
			var args struct {
				Validator common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.GetBalanceGas)
			err := Staker.Native(env.State(), env.BlockContext().Time).RemoveValidator(thor.Address(args.Validator))
			if err != nil {
				panic(err)
			}
			env.UseGas(thor.SstoreSetGas)
			env.UseGas(thor.SstoreResetGas)
			env.Log(validatorRemovedEvent, env.To(), []thor.Bytes32{thor.BytesToBytes32(args.Validator[:])})
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
