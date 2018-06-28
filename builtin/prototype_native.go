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

	events := Prototype.Events()

	mustEventByName := func(name string) *abi.Event {
		if event, found := events.EventByName(name); found {
			return event
		}
		panic("event not found")
	}

	masterEvent := mustEventByName("$Master")
	creditPlanEvent := mustEventByName("$CreditPlan")
	userEvent := mustEventByName("$User")
	sponsorEvent := mustEventByName("$Sponsor")

	defines := []struct {
		name string
		run  func(env *xenv.Environment) []interface{}
	}{
		{"native_master", func(env *xenv.Environment) []interface{} {
			var self common.Address
			env.ParseArgs(&self)

			env.UseGas(thor.GetBalanceGas)
			master := env.State().GetMaster(thor.Address(self))

			return []interface{}{master}
		}},
		{"native_setMaster", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self      common.Address
				NewMaster common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SstoreResetGas)
			env.State().SetMaster(thor.Address(args.Self), thor.Address(args.NewMaster))

			env.Log(masterEvent, thor.Address(args.Self), nil, args.NewMaster)
			return nil
		}},
		{"native_balanceAtBlock", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self        common.Address
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			ctx := env.BlockContext()

			if args.BlockNumber > ctx.Number {
				return []interface{}{&big.Int{}}
			}

			if ctx.Number-args.BlockNumber > thor.MaxBackTrackingBlockNumber {
				return []interface{}{&big.Int{}}
			}

			if args.BlockNumber == ctx.Number {
				env.UseGas(thor.GetBalanceGas)
				val := env.State().GetBalance(thor.Address(args.Self))
				return []interface{}{val}
			}

			env.UseGas(thor.SloadGas)
			blockID := env.Seeker().GetID(args.BlockNumber)

			env.UseGas(thor.SloadGas)
			header := env.Seeker().GetHeader(blockID)

			env.UseGas(thor.SloadGas)
			state := env.State().Spawn(header.StateRoot())

			env.UseGas(thor.GetBalanceGas)
			val := state.GetBalance(thor.Address(args.Self))

			return []interface{}{val}
		}},
		{"native_energyAtBlock", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self        common.Address
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			ctx := env.BlockContext()
			if args.BlockNumber > ctx.Number {
				return []interface{}{&big.Int{}}
			}

			if ctx.Number-args.BlockNumber > thor.MaxBackTrackingBlockNumber {
				return []interface{}{&big.Int{}}
			}

			if args.BlockNumber == ctx.Number {
				env.UseGas(thor.GetBalanceGas)
				val := env.State().GetEnergy(thor.Address(args.Self), ctx.Time)
				return []interface{}{val}
			}

			env.UseGas(thor.SloadGas)
			blockID := env.Seeker().GetID(args.BlockNumber)

			env.UseGas(thor.SloadGas)
			header := env.Seeker().GetHeader(blockID)

			env.UseGas(thor.SloadGas)
			state := env.State().Spawn(header.StateRoot())

			env.UseGas(thor.GetBalanceGas)
			val := state.GetEnergy(thor.Address(args.Self), header.Timestamp())

			return []interface{}{val}
		}},
		{"native_hasCode", func(env *xenv.Environment) []interface{} {
			var self common.Address
			env.ParseArgs(&self)

			env.UseGas(thor.GetBalanceGas)
			hasCode := !env.State().GetCodeHash(thor.Address(self)).IsZero()

			return []interface{}{hasCode}
		}},
		{"native_storageFor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self common.Address
				Key  thor.Bytes32
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			storage := env.State().GetStorage(thor.Address(args.Self), args.Key)
			return []interface{}{storage}
		}},
		{"native_creditPlan", func(env *xenv.Environment) []interface{} {
			var self common.Address
			env.ParseArgs(&self)
			binding := Prototype.Native(env.State()).Bind(thor.Address(self))

			env.UseGas(thor.SloadGas)
			credit, rate := binding.CreditPlan()

			return []interface{}{credit, rate}
		}},
		{"native_setCreditPlan", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self         common.Address
				Credit       *big.Int
				RecoveryRate *big.Int
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SstoreSetGas)
			binding.SetCreditPlan(args.Credit, args.RecoveryRate)
			env.Log(creditPlanEvent, thor.Address(args.Self), nil, args.Credit, args.RecoveryRate)
			return nil
		}},
		{"native_isUser", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self common.Address
				User common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SloadGas)
			isUser := binding.IsUser(thor.Address(args.User))

			return []interface{}{isUser}
		}},
		{"native_userCredit", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self common.Address
				User common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(2 * thor.SloadGas)
			credit := binding.UserCredit(thor.Address(args.User), env.BlockContext().Time)

			return []interface{}{credit}
		}},
		{"native_addUser", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self common.Address
				User common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SloadGas)
			if binding.IsUser(thor.Address(args.User)) {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreSetGas)
			binding.AddUser(thor.Address(args.User), env.BlockContext().Time)

			var action thor.Bytes32
			copy(action[:], "added")
			env.Log(userEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.User[:])}, action)
			return []interface{}{true}
		}},
		{"native_removeUser", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self common.Address
				User common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SloadGas)
			if !binding.IsUser(thor.Address(args.User)) {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreResetGas)
			binding.RemoveUser(thor.Address(args.User))

			var action thor.Bytes32
			copy(action[:], "removed")
			env.Log(userEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.User[:])}, action)
			return []interface{}{true}
		}},
		{"native_sponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self    common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SloadGas)
			if binding.IsSponsor(thor.Address(args.Sponsor)) {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreSetGas)
			binding.Sponsor(thor.Address(args.Sponsor), true)

			var action thor.Bytes32
			copy(action[:], "sponsored")
			env.Log(sponsorEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.Sponsor.Bytes())}, action)
			return []interface{}{true}
		}},
		{"native_unsponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self    common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SloadGas)
			if !binding.IsSponsor(thor.Address(args.Sponsor)) {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreResetGas)
			binding.Sponsor(thor.Address(args.Sponsor), false)

			var action thor.Bytes32
			copy(action[:], "unsponsored")
			env.Log(sponsorEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.Sponsor.Bytes())}, action)
			return []interface{}{true}
		}},
		{"native_isSponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self    common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SloadGas)
			isSponsor := binding.IsSponsor(thor.Address(args.Sponsor))

			return []interface{}{isSponsor}
		}},
		{"native_selectSponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self    common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SloadGas)
			if !binding.IsSponsor(thor.Address(args.Sponsor)) {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreResetGas)
			binding.SelectSponsor(thor.Address(args.Sponsor))

			var action thor.Bytes32
			copy(action[:], "selected")
			env.Log(sponsorEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.Sponsor.Bytes())}, action)

			return []interface{}{true}
		}},
		{"native_currentSponsor", func(env *xenv.Environment) []interface{} {
			var self common.Address
			env.ParseArgs(&self)
			binding := Prototype.Native(env.State()).Bind(thor.Address(self))

			env.UseGas(thor.SloadGas)
			addr := binding.CurrentSponsor()

			return []interface{}{addr}
		}},
	}
	abi := Prototype.NativeABI()
	for _, def := range defines {
		if method, found := abi.MethodByName(def.name); found {
			nativeMethods[methodKey{Prototype.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
