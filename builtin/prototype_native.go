// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/xenv"
)

func init() {
	thorLibABI := Prototype.ThorLibABI

	mustEventByName := func(name string) *abi.Event {
		if event, found := thorLibABI.EventByName(name); found {
			return event
		}
		panic("event not found")
	}
	setMasterEvent := mustEventByName("$SetMaster")
	addRemoveUserEvent := mustEventByName("$AddRemoveUser")
	setUserPlanEvent := mustEventByName("$SetUserPlan")
	sponsorEvent := mustEventByName("$Sponsor")
	selectSponsorEvent := mustEventByName("$SelectSponsor")

	defines := []struct {
		name string
		run  func(env *xenv.Environment) []interface{}
	}{
		{"native_master", func(env *xenv.Environment) []interface{} {
			var target common.Address
			env.ParseArgs(&target)
			binding := Prototype.Native(env.State()).Bind(thor.Address(target))

			env.UseGas(thor.SloadGas)
			master := binding.Master()

			return []interface{}{master}
		}},
		{"native_setMaster", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target    common.Address
				NewMaster common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SstoreResetGas)
			binding.SetMaster(thor.Address(args.NewMaster))
			env.Log(setMasterEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.NewMaster[:])})

			return nil
		}},
		{"native_balanceAtBlock", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target      common.Address
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			ctx := env.BlockContext()
			env.Require(args.BlockNumber <= ctx.Number)

			if args.BlockNumber+thor.MaxBackTrackingBlockNumber < ctx.Number {
				return []interface{}{big.NewInt(0)}
			}

			if args.BlockNumber == ctx.Number {
				env.UseGas(thor.GetBalanceGas)
				val := env.State().GetBalance(thor.Address(args.Target))
				return []interface{}{val}
			}

			env.UseGas(thor.SloadGas)
			blockID := env.Seeker().GetID(args.BlockNumber)
			env.UseGas(3 * thor.SloadGas)
			header := env.Seeker().GetHeader(blockID)
			env.UseGas(thor.SloadGas)
			state := env.State().Spawn(header.StateRoot())
			env.UseGas(thor.GetBalanceGas)
			val := state.GetBalance(thor.Address(args.Target))

			return []interface{}{val}
		}},
		{"native_energyAtBlock", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target      common.Address
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			ctx := env.BlockContext()
			env.Require(args.BlockNumber <= ctx.Number)

			if args.BlockNumber+thor.MaxBackTrackingBlockNumber < ctx.Number {
				return []interface{}{big.NewInt(0)}
			}

			if args.BlockNumber == ctx.Number {
				env.UseGas(thor.GetBalanceGas)
				val := env.State().GetEnergy(thor.Address(args.Target), ctx.Time)
				return []interface{}{val}
			}

			env.UseGas(thor.SloadGas)
			blockID := env.Seeker().GetID(args.BlockNumber)
			env.UseGas(3 * thor.SloadGas)
			header := env.Seeker().GetHeader(blockID)
			env.UseGas(thor.SloadGas)
			state := env.State().Spawn(header.StateRoot())
			env.UseGas(thor.GetBalanceGas)
			val := state.GetEnergy(thor.Address(args.Target), header.Timestamp())

			return []interface{}{val}
		}},
		{"native_hasCode", func(env *xenv.Environment) []interface{} {
			var target common.Address
			env.ParseArgs(&target)

			env.UseGas(thor.SloadGas)
			hasCode := !env.State().GetCodeHash(thor.Address(target)).IsZero()

			return []interface{}{hasCode}
		}},
		{"native_storage", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target common.Address
				Key    thor.Bytes32
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			val := env.State().GetStorage(thor.Address(args.Target), args.Key)

			return []interface{}{val}
		}},
		{"native_storageAtBlock", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target      common.Address
				Key         thor.Bytes32
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			ctx := env.BlockContext()
			env.Require(args.BlockNumber <= ctx.Number)

			if args.BlockNumber+thor.MaxBackTrackingBlockNumber < ctx.Number {
				return []interface{}{thor.Bytes32{}}
			}

			if args.BlockNumber == ctx.Number {
				env.UseGas(thor.SloadGas)
				val := env.State().GetStorage(thor.Address(args.Target), args.Key)
				return []interface{}{val}
			}

			env.UseGas(thor.SloadGas)
			blockID := env.Seeker().GetID(args.BlockNumber)
			env.UseGas(3 * thor.SloadGas)
			header := env.Seeker().GetHeader(blockID)
			env.UseGas(thor.SloadGas)
			state := env.State().Spawn(header.StateRoot())
			env.UseGas(thor.SloadGas)
			val := state.GetStorage(thor.Address(args.Target), args.Key)

			return []interface{}{val}
		}},
		{"native_userPlan", func(env *xenv.Environment) []interface{} {
			var target common.Address
			env.ParseArgs(&target)
			binding := Prototype.Native(env.State()).Bind(thor.Address(target))

			env.UseGas(thor.SloadGas)
			credit, rate := binding.UserPlan()

			return []interface{}{credit, rate}
		}},
		{"native_setUserPlan", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target       common.Address
				Credit       *big.Int
				RecoveryRate *big.Int
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SstoreSetGas)
			binding.SetUserPlan(args.Credit, args.RecoveryRate)
			env.Log(setUserPlanEvent, thor.Address(args.Target), nil, args.Credit, args.RecoveryRate)

			return nil
		}},
		{"native_isUser", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target common.Address
				User   common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SloadGas)
			isUser := binding.IsUser(thor.Address(args.User))

			return []interface{}{isUser}
		}},
		{"native_userCredit", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target common.Address
				User   common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SloadGas)
			credit := binding.UserCredit(thor.Address(args.User), env.BlockContext().Time)

			return []interface{}{credit}
		}},
		{"native_addUser", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target common.Address
				User   common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SloadGas)
			env.Require(binding.AddUser(thor.Address(args.User), env.BlockContext().Time))
			env.UseGas(thor.SstoreSetGas)
			env.Log(addRemoveUserEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.User[:])}, true)

			return nil
		}},
		{"native_removeUser", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target common.Address
				User   common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SloadGas)
			env.Require(binding.RemoveUser(thor.Address(args.User)))
			env.UseGas(thor.SstoreResetGas)
			env.Log(addRemoveUserEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.User[:])}, false)

			return nil
		}},
		{"native_sponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target  common.Address
				Caller  common.Address
				YesOrNo bool
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SloadGas)
			env.Require(binding.Sponsor(thor.Address(args.Caller), args.YesOrNo))
			if args.YesOrNo {
				env.UseGas(thor.SstoreSetGas)
			} else {
				env.UseGas(thor.SstoreResetGas)
			}
			env.Log(sponsorEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.Caller.Bytes())}, args.YesOrNo)

			return nil
		}},
		{"native_isSponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target  common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SloadGas)
			b := binding.IsSponsor(thor.Address(args.Sponsor))

			return []interface{}{b}
		}},
		{"native_selectSponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Target  common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Target))

			env.UseGas(thor.SloadGas)
			env.Require(binding.SelectSponsor(thor.Address(args.Sponsor)))
			env.UseGas(thor.SstoreResetGas)
			env.Log(selectSponsorEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.Sponsor[:])})

			return nil
		}},
		{"native_currentSponsor", func(env *xenv.Environment) []interface{} {
			var target common.Address
			env.ParseArgs(&target)
			binding := Prototype.Native(env.State()).Bind(thor.Address(target))

			env.UseGas(thor.SloadGas)
			addr := binding.CurrentSponsor()

			return []interface{}{addr}
		}},
	}
	nativeABI := Prototype.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[methodKey{Prototype.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
