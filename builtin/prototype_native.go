// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/xenv"
)

func init() {

	decodeBytes32 := func(data []byte) thor.Bytes32 {
		if len(data) == 0 {
			return thor.Bytes32{}
		}
		var b []byte

		if err := rlp.DecodeBytes(data, &b); err != nil {
			return thor.Bytes32{}
		}
		return thor.BytesToBytes32(b)
	}

	eventLibABI := Prototype.EventABI

	mustEventByName := func(name string) *abi.Event {
		if event, found := eventLibABI.EventByName(name); found {
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
			var self common.Address
			env.ParseArgs(&self)
			binding := Prototype.Native(env.State()).Bind(thor.Address(self))

			env.UseGas(thor.GetBalanceGas)
			master := binding.Master()

			return []interface{}{master}
		}},
		{"native_setMaster", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self      common.Address
				NewMaster common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SstoreResetGas)

			binding.SetMaster(thor.Address(args.NewMaster))
			env.Log(setMasterEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.NewMaster[:])})

			return nil
		}},
		{"native_balanceAtBlock", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self        common.Address
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			ctx := env.BlockContext()
			env.Must(args.BlockNumber <= ctx.Number)

			if args.BlockNumber+thor.MaxBackTrackingBlockNumber < ctx.Number {
				return []interface{}{big.NewInt(0)}
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
			env.Must(args.BlockNumber <= ctx.Number)

			if args.BlockNumber+thor.MaxBackTrackingBlockNumber < ctx.Number {
				return []interface{}{big.NewInt(0)}
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
			data := env.State().GetRawStorage(thor.Address(args.Self), args.Key)
			return []interface{}{decodeBytes32(data)}
		}},
		{"native_userPlan", func(env *xenv.Environment) []interface{} {
			var self common.Address
			env.ParseArgs(&self)
			binding := Prototype.Native(env.State()).Bind(thor.Address(self))

			env.UseGas(thor.SloadGas)
			credit, rate := binding.UserPlan()

			return []interface{}{credit, rate}
		}},
		{"native_setUserPlan", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self         common.Address
				Credit       *big.Int
				RecoveryRate *big.Int
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SstoreSetGas)
			binding.SetUserPlan(args.Credit, args.RecoveryRate)
			env.Log(setUserPlanEvent, thor.Address(args.Self), nil, args.Credit, args.RecoveryRate)

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
			env.UseGas(thor.SstoreSetGas)
			binding.AddUser(thor.Address(args.User), env.BlockContext().Time)
			env.Log(addRemoveUserEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.User[:])}, true)

			return nil
		}},
		{"native_removeUser", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self common.Address
				User common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SstoreResetGas)
			binding.RemoveUser(thor.Address(args.User))
			env.Log(addRemoveUserEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.User[:])}, false)

			return nil
		}},
		{"native_sponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self    common.Address
				Caller  common.Address
				YesOrNo bool
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			if args.YesOrNo {
				env.UseGas(thor.SstoreSetGas)
			} else {
				env.UseGas(thor.SstoreResetGas)
			}
			binding.Sponsor(thor.Address(args.Caller), args.YesOrNo)
			env.Log(sponsorEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.Caller.Bytes())}, args.YesOrNo)

			return nil
		}},
		{"native_isSponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self    common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SloadGas)
			b := binding.IsSponsor(thor.Address(args.Sponsor))

			return []interface{}{b}
		}},
		{"native_selectSponsor", func(env *xenv.Environment) []interface{} {
			var args struct {
				Self    common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.State()).Bind(thor.Address(args.Self))

			env.UseGas(thor.SstoreResetGas)
			binding.SelectSponsor(thor.Address(args.Sponsor))
			env.Log(selectSponsorEvent, thor.Address(args.Self), []thor.Bytes32{thor.BytesToBytes32(args.Sponsor[:])})

			return nil
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
