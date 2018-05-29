// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/pkg/errors"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm/evm"
)

type nativeMethod struct {
	abi *abi.Method
	run func(env *environment) []interface{}
}

type methodKey struct {
	thor.Address
	abi.MethodID
}

const (
	blake2b256WordGas uint64 = 3
	blake2b256Gas     uint64 = 15
)

var privateMethods = make(map[methodKey]*nativeMethod)

func initParamsMethods() {
	defines := []struct {
		name string
		run  func(env *environment) []interface{}
	}{
		{"native_getExecutor", func(env *environment) []interface{} {
			env.UseGas(ethparams.SloadGas)
			return []interface{}{Executor.Address}
		}},
		{"native_get", func(env *environment) []interface{} {
			var key common.Hash
			env.ParseArgs(&key)
			env.UseGas(ethparams.SloadGas)
			v := Params.Native(env.state).Get(thor.Bytes32(key))
			return []interface{}{v}
		}},
		{"native_set", func(env *environment) []interface{} {
			var args struct {
				Key   common.Hash
				Value *big.Int
			}
			env.ParseArgs(&args)
			env.UseGas(ethparams.SstoreSetGas)
			Params.Native(env.state).Set(thor.Bytes32(args.Key), args.Value)
			return nil
		}},
	}
	nativeABI := Params.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[methodKey{Params.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}

func initAuthorityMethods() {
	defines := []struct {
		name string
		run  func(env *environment) []interface{}
	}{
		{"native_getExecutor", func(env *environment) []interface{} {
			env.UseGas(ethparams.SloadGas)
			return []interface{}{Executor.Address}
		}},
		{"native_add", func(env *environment) []interface{} {
			var args struct {
				Signer   common.Address
				Endorsor common.Address
				Identity common.Hash
			}
			env.ParseArgs(&args)
			env.UseGas(ethparams.SloadGas)
			ok := Authority.Native(env.state).Add(thor.Address(args.Signer), thor.Address(args.Endorsor), thor.Bytes32(args.Identity))
			if ok {
				env.UseGas(ethparams.SstoreSetGas + ethparams.SstoreResetGas)
			}
			return []interface{}{ok}
		}},
		{"native_remove", func(env *environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)
			env.UseGas(ethparams.SloadGas)
			ok := Authority.Native(env.state).Remove(thor.Address(signer))
			if ok {
				env.UseGas(ethparams.SstoreClearGas + ethparams.SstoreResetGas*2)
			}
			return []interface{}{ok}
		}},
		{"native_get", func(env *environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)
			env.UseGas(ethparams.SloadGas * 3)
			p := Authority.Native(env.state).Get(thor.Address(signer))
			return []interface{}{!p.IsEmpty(), p.Endorsor, p.Identity, p.Active}
		}},
		{"native_first", func(env *environment) []interface{} {
			env.UseGas(ethparams.SloadGas)
			signer := Authority.Native(env.state).First()
			return []interface{}{signer}
		}},
		{"native_next", func(env *environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)
			env.UseGas(ethparams.SloadGas * 4)
			p := Authority.Native(env.state).Get(thor.Address(signer))
			var next thor.Address
			if p.Next != nil {
				next = *p.Next
			}
			return []interface{}{next}
		}},
		{"native_isEndorsed", func(env *environment) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)
			env.UseGas(ethparams.SloadGas * 2)
			p := Authority.Native(env.state).Get(thor.Address(signer))
			if p.IsEmpty() {
				return []interface{}{false}
			}
			bal := env.state.GetBalance(p.Endorsor)
			endorsement := Params.Native(env.state).Get(thor.KeyProposerEndorsement)
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

func initEnergyMethods() {
	defines := []struct {
		name string
		run  func(env *environment) []interface{}
	}{
		{"native_getTotalSupply", func(env *environment) []interface{} {
			env.UseGas(ethparams.SloadGas)
			supply := Energy.Native(env.state).GetTotalSupply(env.BlockTime())
			return []interface{}{supply}
		}},
		{"native_getTotalBurned", func(env *environment) []interface{} {
			env.UseGas(ethparams.SloadGas)
			burned := Energy.Native(env.state).GetTotalBurned()
			return []interface{}{burned}
		}},
		{"native_getBalance", func(env *environment) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)
			env.UseGas(ethparams.SloadGas)
			bal := Energy.Native(env.state).GetBalance(thor.Address(addr), env.BlockTime())
			return []interface{}{bal}
		}},
		{"native_addBalance", func(env *environment) []interface{} {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)
			if env.state.Exists(thor.Address(args.Addr)) {
				env.UseGas(ethparams.SstoreResetGas)
			} else {
				env.UseGas(ethparams.SstoreSetGas)
			}
			Energy.Native(env.state).AddBalance(thor.Address(args.Addr), args.Amount, env.BlockTime())
			return nil
		}},
		{"native_subBalance", func(env *environment) []interface{} {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)
			env.UseGas(ethparams.SloadGas)
			ok := Energy.Native(env.state).SubBalance(thor.Address(args.Addr), args.Amount, env.BlockTime())
			if ok {
				env.UseGas(ethparams.SstoreResetGas)
			}
			return []interface{}{ok}
		}},
	}
	nativeABI := Energy.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[methodKey{Energy.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}

func initPrototypeMethods() {
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
		run  func(env *environment) []interface{}
	}{
		{"native_master", func(env *environment) []interface{} {
			var target common.Address
			env.ParseArgs(&target)
			binding := Prototype.Native(env.state).Bind(thor.Address(target))

			master := binding.Master()
			env.UseGas(ethparams.SloadGas)

			return []interface{}{master}
		}},
		{"native_setMaster", func(env *environment) []interface{} {
			var args struct {
				Target    common.Address
				NewMaster common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			binding.SetMaster(thor.Address(args.NewMaster))
			env.UseGas(ethparams.SstoreResetGas)
			env.Log(setMasterEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.NewMaster[:])})

			return nil
		}},
		{"native_balanceAtBlock", func(env *environment) []interface{} {
			var args struct {
				Target      common.Address
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			env.Require(big.NewInt(int64(args.BlockNumber)).Cmp(env.evm.BlockNumber) < 1)

			// BlockNumber + MaxBackTrackingBlockNumber < current blocknumber
			if big.NewInt(int64(args.BlockNumber)+thor.MaxBackTrackingBlockNumber).Cmp(env.evm.BlockNumber) == -1 {
				return []interface{}{0}
			}

			// blocknumber == current blocknumber
			if big.NewInt(int64(args.BlockNumber)).Cmp(env.evm.BlockNumber) == 0 {
				val := env.state.GetBalance(thor.Address(args.Target))
				env.UseGas(ethparams.SloadGas)
				return []interface{}{val}
			}

			blockID := env.seeker.GetID(args.BlockNumber)
			env.UseGas(ethparams.SloadGas)
			header := env.seeker.GetHeader(blockID)
			env.UseGas(3 * ethparams.SloadGas)
			state := env.state.Spawn(header.StateRoot())
			env.UseGas(ethparams.SloadGas)
			val := state.GetBalance(thor.Address(args.Target))
			env.UseGas(ethparams.SloadGas)

			return []interface{}{val}
		}},
		{"native_energyAtBlock", func(env *environment) []interface{} {
			var args struct {
				Target      common.Address
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			env.Require(big.NewInt(int64(args.BlockNumber)).Cmp(env.evm.BlockNumber) < 1)

			// BlockNumber + MaxBackTrackingBlockNumber < current blocknumber
			if big.NewInt(int64(args.BlockNumber)+thor.MaxBackTrackingBlockNumber).Cmp(env.evm.BlockNumber) == -1 {
				return []interface{}{0}
			}

			// blocknumber == current blocknumber
			if big.NewInt(int64(args.BlockNumber)).Cmp(env.evm.BlockNumber) == 0 {
				val := env.state.GetEnergy(thor.Address(args.Target), env.BlockTime())
				env.UseGas(ethparams.SloadGas)
				return []interface{}{val}
			}

			blockID := env.seeker.GetID(args.BlockNumber)
			env.UseGas(ethparams.SloadGas)
			header := env.seeker.GetHeader(blockID)
			env.UseGas(3 * ethparams.SloadGas)
			state := env.state.Spawn(header.StateRoot())
			env.UseGas(ethparams.SloadGas)
			val := state.GetEnergy(thor.Address(args.Target), header.Timestamp())
			env.UseGas(ethparams.SloadGas)

			return []interface{}{val}
		}},
		{"native_hasCode", func(env *environment) []interface{} {
			var target common.Address
			env.ParseArgs(&target)

			hasCode := !env.state.GetCodeHash(thor.Address(target)).IsZero()
			env.UseGas(ethparams.SloadGas)

			return []interface{}{hasCode}
		}},
		{"native_storage", func(env *environment) []interface{} {
			var args struct {
				Target common.Address
				Key    thor.Bytes32
			}
			env.ParseArgs(&args)

			val := env.state.GetStorage(thor.Address(args.Target), args.Key)
			env.UseGas(ethparams.SloadGas)

			return []interface{}{val}
		}},
		{"native_storageAtBlock", func(env *environment) []interface{} {
			var args struct {
				Target      common.Address
				Key         thor.Bytes32
				BlockNumber uint32
			}
			env.ParseArgs(&args)
			env.Require(big.NewInt(int64(args.BlockNumber)).Cmp(env.evm.BlockNumber) < 1)

			// BlockNumber + MaxBackTrackingBlockNumber < current blocknumber
			if big.NewInt(int64(args.BlockNumber)+thor.MaxBackTrackingBlockNumber).Cmp(env.evm.BlockNumber) == -1 {
				return []interface{}{0}
			}

			// blocknumber == current blocknumber
			if big.NewInt(int64(args.BlockNumber)).Cmp(env.evm.BlockNumber) == 0 {
				val := env.state.GetStorage(thor.Address(args.Target), args.Key)
				env.UseGas(ethparams.SloadGas)
				return []interface{}{val}
			}

			blockID := env.seeker.GetID(args.BlockNumber)
			env.UseGas(ethparams.SloadGas)
			header := env.seeker.GetHeader(blockID)
			env.UseGas(3 * ethparams.SloadGas)
			state := env.state.Spawn(header.StateRoot())
			env.UseGas(ethparams.SloadGas)
			val := state.GetStorage(thor.Address(args.Target), args.Key)
			env.UseGas(ethparams.SloadGas)

			return []interface{}{val}
		}},
		{"native_userPlan", func(env *environment) []interface{} {
			var target common.Address
			env.ParseArgs(&target)
			binding := Prototype.Native(env.state).Bind(thor.Address(target))

			credit, rate := binding.UserPlan()
			env.UseGas(ethparams.SloadGas)

			return []interface{}{credit, rate}
		}},
		{"native_setUserPlan", func(env *environment) []interface{} {
			var args struct {
				Target       common.Address
				Credit       *big.Int
				RecoveryRate *big.Int
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			binding.SetUserPlan(args.Credit, args.RecoveryRate)
			env.UseGas(ethparams.SstoreSetGas)
			env.Log(setUserPlanEvent, thor.Address(args.Target), nil, args.Credit, args.RecoveryRate)

			return nil
		}},
		{"native_isUser", func(env *environment) []interface{} {
			var args struct {
				Target common.Address
				User   common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			isUser := binding.IsUser(thor.Address(args.User))
			env.UseGas(ethparams.SloadGas)

			return []interface{}{isUser}
		}},
		{"native_userCredit", func(env *environment) []interface{} {
			var args struct {
				Target common.Address
				User   common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			credit := binding.UserCredit(thor.Address(args.User), env.BlockTime())
			env.UseGas(ethparams.SloadGas)

			return []interface{}{credit}
		}},
		{"native_addUser", func(env *environment) []interface{} {
			var args struct {
				Target common.Address
				User   common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			env.UseGas(ethparams.SloadGas)
			env.Require(binding.AddUser(thor.Address(args.User), env.BlockTime()))
			env.UseGas(ethparams.SstoreSetGas)
			env.Log(addRemoveUserEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.User[:])}, true)

			return nil
		}},
		{"native_removeUser", func(env *environment) []interface{} {
			var args struct {
				Target common.Address
				User   common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			env.UseGas(ethparams.SloadGas)
			env.Require(binding.RemoveUser(thor.Address(args.User)))
			env.UseGas(ethparams.SstoreClearGas)
			env.Log(addRemoveUserEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.User[:])}, false)

			return nil
		}},
		{"native_sponsor", func(env *environment) []interface{} {
			var args struct {
				Target  common.Address
				Caller  common.Address
				YesOrNo bool
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			env.UseGas(ethparams.SloadGas)
			env.Require(binding.Sponsor(thor.Address(args.Caller), args.YesOrNo))
			if args.YesOrNo {
				env.UseGas(ethparams.SstoreSetGas)
			} else {
				env.UseGas(ethparams.SstoreClearGas)
			}
			env.Log(sponsorEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.Caller.Bytes())}, args.YesOrNo)

			return nil
		}},
		{"native_isSponsor", func(env *environment) []interface{} {
			var args struct {
				Target  common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			b := binding.IsSponsor(thor.Address(args.Sponsor))
			env.UseGas(ethparams.SloadGas)

			return []interface{}{b}
		}},
		{"native_selectSponsor", func(env *environment) []interface{} {
			var args struct {
				Target  common.Address
				Sponsor common.Address
			}
			env.ParseArgs(&args)
			binding := Prototype.Native(env.state).Bind(thor.Address(args.Target))

			env.UseGas(ethparams.SloadGas)
			env.Require(binding.SelectSponsor(thor.Address(args.Sponsor)))
			env.UseGas(ethparams.SstoreResetGas)
			env.Log(selectSponsorEvent, thor.Address(args.Target), []thor.Bytes32{thor.BytesToBytes32(args.Sponsor[:])})

			return nil
		}},
		{"native_currentSponsor", func(env *environment) []interface{} {
			var target common.Address
			env.ParseArgs(&target)
			binding := Prototype.Native(env.state).Bind(thor.Address(target))

			addr := binding.CurrentSponsor()
			env.UseGas(ethparams.SloadGas)

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

func initExtensionMethods() {
	defines := []struct {
		name string
		run  func(env *environment) []interface{}
	}{
		{"native_blake2b256", func(env *environment) []interface{} {
			var data []byte
			env.ParseArgs(&data)
			env.UseGas(uint64(len(data)+31)/32*blake2b256WordGas + blake2b256Gas)
			output := Extension.Native(env.state, env.seeker).Blake2b256(data)
			return []interface{}{output}
		}},
		{"native_getBlockIDByNum", func(env *environment) []interface{} {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			env.UseGas(ethparams.SloadGas)
			output := Extension.Native(env.state, env.seeker).GetBlockIDByNum(blockNum)
			return []interface{}{output}
		}},
		{"native_getTotalScoreByNum", func(env *environment) []interface{} {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			env.UseGas(ethparams.SloadGas)
			header := Extension.Native(env.state, env.seeker).GetBlockHeaderByNum(blockNum)
			return []interface{}{header.TotalScore()}
		}},
		{"native_getTimestampByNum", func(env *environment) []interface{} {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			env.UseGas(ethparams.SloadGas)
			header := Extension.Native(env.state, env.seeker).GetBlockHeaderByNum(blockNum)
			return []interface{}{header.Timestamp()}
		}},
		{"native_getProposerByNum", func(env *environment) []interface{} {
			var blockNum uint32
			env.ParseArgs(&blockNum)
			env.UseGas(ethparams.SloadGas)
			header := Extension.Native(env.state, env.seeker).GetBlockHeaderByNum(blockNum)
			proposer, _ := header.Signer()
			return []interface{}{proposer}
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

func init() {
	initParamsMethods()
	initAuthorityMethods()
	initEnergyMethods()
	initPrototypeMethods()
	initExtensionMethods()
}

// HandleNativeCall entry of native methods implementation.
func HandleNativeCall(
	seeker *chain.Seeker,
	state *state.State,
	vm *evm.EVM,
	contract *evm.Contract,
	readonly bool,
	txEnv *TransactionEnv,
) func() ([]byte, error) {
	methodID, err := abi.ExtractMethodID(contract.Input)
	if err != nil {
		return nil
	}

	var method *nativeMethod
	if contract.Address() == contract.Caller() {
		// private methods require caller == to
		method = privateMethods[methodKey{thor.Address(contract.Address()), methodID}]
	}

	if method == nil {
		return nil
	}

	if readonly && !method.abi.Const() {
		return func() ([]byte, error) {
			return nil, evm.ErrWriteProtection()
		}
	}
	if contract.Value().Sign() != 0 {
		// all private and prototype methods are not payable
		return func() ([]byte, error) {
			return nil, evm.ErrExecutionReverted()
		}
	}

	env := newEnvironment(method.abi, seeker, state, vm, contract, txEnv)
	return func() (data []byte, err error) {
		defer func() {
			if e := recover(); e != nil {
				if rec, ok := e.(*vmError); ok {
					err = rec.cause
				} else {
					panic(e)
				}
			}
		}()
		output := method.run(env)
		data, err = method.abi.EncodeOutput(output...)
		if err != nil {
			panic(errors.WithMessage(err, "encode native output"))
		}
		return
	}
}
