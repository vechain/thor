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
			master, err := env.State().GetMaster(thor.Address(self))
			if err != nil {
				panic(err)
			}

			return []interface{}{master}
		}},
		{"native_setMaster", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self      common.Address
				NewMaster common.Address
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SstoreResetGas)
			if err := env.State().SetMaster(thor.Address(args.Self), thor.Address(args.NewMaster)); err != nil {
				panic(err)
			}

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

			if ctx.Number-args.BlockNumber > thor.MaxStateHistory {
				return []interface{}{&big.Int{}}
			}

			if args.BlockNumber == ctx.Number {
				env.UseGas(thor.GetBalanceGas)
				val, err := env.State().GetBalance(thor.Address(args.Self))
				if err != nil {
					panic(err)
				}
				return []interface{}{val}
			}

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)
			summary, err := env.Chain().GetBlockSummary(args.BlockNumber)
			if err != nil {
				panic(err)
			}

			env.UseGas(thor.SloadGas)
			state := env.State().Checkout(summary.Header.StateRoot(), summary.Header.Number(), summary.Conflicts, summary.SteadyNum)

			env.UseGas(thor.GetBalanceGas)
			val, err := state.GetBalance(thor.Address(args.Self))
			if err != nil {
				panic(err)
			}

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

			if ctx.Number-args.BlockNumber > thor.MaxStateHistory {
				return []interface{}{&big.Int{}}
			}

			if args.BlockNumber == ctx.Number {
				env.UseGas(thor.GetBalanceGas)
				val, err := env.State().GetEnergy(thor.Address(args.Self), ctx.Time)
				if err != nil {
					panic(err)
				}
				return []interface{}{val}
			}

			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)
			summary, err := env.Chain().GetBlockSummary(args.BlockNumber)
			if err != nil {
				panic(err)
			}

			env.UseGas(thor.SloadGas)
			state := env.State().Checkout(summary.Header.StateRoot(), summary.Header.Number(), summary.Conflicts, summary.SteadyNum)

			env.UseGas(thor.GetBalanceGas)
			val, err := state.GetEnergy(thor.Address(args.Self), summary.Header.Timestamp())
			if err != nil {
				panic(err)
			}

			return []interface{}{val}
		}},
		{"native_hasCode", func(env *xenv.Environment) []interface{} {
			var self common.Address
			env.ParseArgs(&self)

			env.UseGas(thor.GetBalanceGas)
			codeHash, err := env.State().GetCodeHash(thor.Address(self))
			if err != nil {
				panic(err)
			}
			hasCode := !codeHash.IsZero()

			return []interface{}{hasCode}
		}},
		{"native_storageFor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self common.Address
				Key  thor.Bytes32
			}
			env.ParseArgs(&args)

			env.UseGas(thor.SloadGas)
			storage, err := env.State().GetStorage(thor.Address(args.Self), args.Key)
			if err != nil {
				panic(err)
			}
			return []interface{}{storage}
		}},
		{"native_creditPlan", func(env *xenv.Environment) []interface{} {
			var self common.Address
			env.ParseArgs(&self)
			binding := Prototype.Native(env.State()).Bind(thor.Address(self))

			env.UseGas(thor.SloadGas)
			credit, rate, err := binding.CreditPlan()
			if err != nil {
				panic(err)
			}

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
			if err := binding.SetCreditPlan(args.Credit, args.RecoveryRate); err != nil {
				panic(err)
			}
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
			isUser, err := binding.IsUser(thor.Address(args.User))
			if err != nil {
				panic(err)
			}

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
			credit, err := binding.UserCredit(thor.Address(args.User), env.BlockContext().Time)
			if err != nil {
				panic(err)
			}

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
			isUser, err := binding.IsUser(thor.Address(args.User))
			if err != nil {
				panic(err)
			}
			if isUser {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreSetGas)
			if err := binding.AddUser(thor.Address(args.User), env.BlockContext().Time); err != nil {
				panic(err)
			}

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
			isUser, err := binding.IsUser(thor.Address(args.User))
			if err != nil {
				panic(err)
			}
			if !isUser {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreResetGas)
			if err := binding.RemoveUser(thor.Address(args.User)); err != nil {
				panic(err)
			}

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
			isSponsor, err := binding.IsSponsor(thor.Address(args.Sponsor))
			if err != nil {
				panic(err)
			}
			if isSponsor {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreSetGas)
			if err := binding.Sponsor(thor.Address(args.Sponsor), true); err != nil {
				panic(err)
			}

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
			isSponsor, err := binding.IsSponsor(thor.Address(args.Sponsor))
			if err != nil {
				panic(err)
			}
			if !isSponsor {
				return []interface{}{false}
			}

			env.UseGas(thor.SstoreResetGas)
			if err := binding.Sponsor(thor.Address(args.Sponsor), false); err != nil {
				panic(err)
			}

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
			isSponsor, err := binding.IsSponsor(thor.Address(args.Sponsor))
			if err != nil {
				panic(err)
			}

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
			isSponsor, err := binding.IsSponsor(thor.Address(args.Sponsor))
			if err != nil {
				panic(err)
			}
			if !isSponsor {
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
			addr, err := binding.CurrentSponsor()
			if err != nil {
				panic(err)
			}

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
