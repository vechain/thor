package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin/prototype"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm/evm"
)

type addressAndMethodID struct {
	thor.Address
	abi.MethodID
}

var (
	privateMethods   = make(map[addressAndMethodID]*nativeMethod)
	prototypeMethods = make(map[abi.MethodID]*nativeMethod)
)

func initParamsMethods() {
	defines := []struct {
		name string
		run  func(env *bridge) []interface{}
	}{
		{"native_getExecutor", func(env *bridge) []interface{} {
			env.UseGas(ethparams.SloadGas)
			return []interface{}{Executor.Address}
		}},
		{"native_get", func(env *bridge) []interface{} {
			var key common.Hash
			env.ParseArgs(&key)
			env.UseGas(ethparams.SloadGas)
			v := Params.Native(env.State).Get(thor.Bytes32(key))
			return []interface{}{v}
		}},
		{"native_set", func(env *bridge) []interface{} {
			var args struct {
				Key   common.Hash
				Value *big.Int
			}
			env.ParseArgs(&args)
			env.UseGas(ethparams.SstoreSetGas)
			Params.Native(env.State).Set(thor.Bytes32(args.Key), args.Value)
			return nil
		}},
	}
	nativeABI := Params.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[addressAndMethodID{Params.Address, method.ID()}] = &nativeMethod{
				ABI: method,
				Run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}

func initAuthorityMethods() {
	defines := []struct {
		name string
		run  func(env *bridge) []interface{}
	}{
		{"native_getExecutor", func(env *bridge) []interface{} {
			env.UseGas(ethparams.SloadGas)
			return []interface{}{Executor.Address}
		}},
		{"native_add", func(env *bridge) []interface{} {
			var args struct {
				Signer   common.Address
				Endorsor common.Address
				Identity common.Hash
			}
			env.ParseArgs(&args)
			env.UseGas(ethparams.SloadGas)
			ok := Authority.Native(env.State).Add(thor.Address(args.Signer), thor.Address(args.Endorsor), thor.Bytes32(args.Identity))
			if ok {
				env.UseGas(ethparams.SstoreSetGas + ethparams.SstoreResetGas)
			}
			return []interface{}{ok}
		}},
		{"native_remove", func(env *bridge) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)
			env.UseGas(ethparams.SloadGas)
			ok := Authority.Native(env.State).Remove(thor.Address(signer))
			if ok {
				env.UseGas(ethparams.SstoreClearGas + ethparams.SstoreResetGas*2)
			}
			return []interface{}{ok}
		}},
		{"native_get", func(env *bridge) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)
			env.UseGas(ethparams.SloadGas * 3)
			p := Authority.Native(env.State).Get(thor.Address(signer))
			return []interface{}{!p.IsEmpty(), p.Endorsor, p.Identity, p.Active}
		}},
		{"native_first", func(env *bridge) []interface{} {
			env.UseGas(ethparams.SloadGas)
			signer := Authority.Native(env.State).First()
			return []interface{}{signer}
		}},
		{"native_next", func(env *bridge) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)
			env.UseGas(ethparams.SloadGas * 4)
			p := Authority.Native(env.State).Get(thor.Address(signer))
			var next thor.Address
			if p.Next != nil {
				next = *p.Next
			}
			return []interface{}{next}
		}},
		{"native_isEndorsed", func(env *bridge) []interface{} {
			var signer common.Address
			env.ParseArgs(&signer)
			env.UseGas(ethparams.SloadGas * 2)
			p := Authority.Native(env.State).Get(thor.Address(signer))
			if p.IsEmpty() {
				return []interface{}{false}
			}
			bal := env.State.GetBalance(p.Endorsor)
			endorsement := Params.Native(env.State).Get(thor.KeyProposerEndorsement)
			return []interface{}{bal.Cmp(endorsement) >= 0}
		}},
	}
	nativeABI := Authority.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[addressAndMethodID{Authority.Address, method.ID()}] = &nativeMethod{
				ABI: method,
				Run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}

func initEnergyMethods() {
	defines := []struct {
		name string
		run  func(env *bridge) []interface{}
	}{
		{"native_getTotalSupply", func(env *bridge) []interface{} {
			env.UseGas(ethparams.SloadGas)
			supply := Energy.Native(env.State).GetTotalSupply(env.BlockTime())
			return []interface{}{supply}
		}},
		{"native_getTotalBurned", func(env *bridge) []interface{} {
			env.UseGas(ethparams.SloadGas)
			burned := Energy.Native(env.State).GetTotalBurned()
			return []interface{}{burned}
		}},
		{"native_getBalance", func(env *bridge) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)
			env.UseGas(ethparams.SloadGas)
			bal := Energy.Native(env.State).GetBalance(thor.Address(addr), env.BlockTime())
			return []interface{}{bal}
		}},
		{"native_addBalance", func(env *bridge) []interface{} {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)
			if env.State.Exists(thor.Address(args.Addr)) {
				env.UseGas(ethparams.SstoreResetGas)
			} else {
				env.UseGas(ethparams.SstoreSetGas)
			}
			Energy.Native(env.State).AddBalance(thor.Address(args.Addr), args.Amount, env.BlockTime())
			return nil
		}},
		{"native_subBalance", func(env *bridge) []interface{} {
			var args struct {
				Addr   common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)
			env.UseGas(ethparams.SloadGas)
			ok := Energy.Native(env.State).SubBalance(thor.Address(args.Addr), args.Amount, env.BlockTime())
			if ok {
				env.UseGas(ethparams.SstoreResetGas)
			}
			return []interface{}{ok}
		}},
	}
	nativeABI := Energy.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[addressAndMethodID{Energy.Address, method.ID()}] = &nativeMethod{
				ABI: method,
				Run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}

func initPrototypeMethods() {
	defines := []struct {
		name string
		run  func(env *bridge) []interface{}
	}{
		{"native_contractify", func(env *bridge) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)

			env.UseGas(ethparams.SloadGas)
			env.Require(env.VM.Contractify(addr))
			return nil
		}},
	}
	nativeABI := Prototype.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[addressAndMethodID{Prototype.Address, method.ID()}] = &nativeMethod{
				ABI: method,
				Run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}

func initPrototypeInterfaceMethods() {
	nativeABI := Prototype.InterfaceABI

	mustEventByName := func(name string) *abi.Event {
		if event, found := nativeABI.EventByName(name); found {
			return event
		}
		panic("event not found")
	}
	setMasterEvent := mustEventByName("$SetMaster")
	addRemoveUserEvent := mustEventByName("$AddRemoveUser")
	setUserPlanEvent := mustEventByName("$SetUserPlan")
	sponsorEvent := mustEventByName("$Sponsor")
	selectSponsorEvent := mustEventByName("$SelectSponsor")

	energyTransferMethod, ok := Energy.ABI.MethodByName("transfer")
	if !ok {
		panic("transfer method not found")
	}

	defines := []struct {
		name string
		run  func(env *bridge, binding *prototype.Binding) []interface{}
	}{
		{"$master", func(env *bridge, binding *prototype.Binding) []interface{} {
			env.UseGas(ethparams.SloadGas)
			master := binding.Master()
			return []interface{}{master}
		}},
		{"$set_master", func(env *bridge, binding *prototype.Binding) []interface{} {
			var newMaster common.Address
			env.ParseArgs(&newMaster)
			env.UseGas(ethparams.SloadGas)
			// master or account itself is allowed
			env.Require(binding.Master() == env.Caller() || env.Caller() == env.To())
			env.UseGas(ethparams.SstoreResetGas)
			binding.SetMaster(thor.Address(newMaster))
			env.Log(setMasterEvent, []thor.Bytes32{thor.BytesToBytes32(newMaster[:])})
			return nil
		}},
		{"$has_code", func(env *bridge, binding *prototype.Binding) []interface{} {
			if env.VM.IsContractified(common.Address(env.To())) {
				return []interface{}{false}
			}
			hasCode := !env.State.GetCodeHash(env.To()).IsZero()
			return []interface{}{hasCode}
		}},
		{"$energy", func(env *bridge, binding *prototype.Binding) []interface{} {
			env.UseGas(ethparams.SloadGas)
			bal := Energy.Native(env.State).GetBalance(env.To(), env.BlockTime())
			return []interface{}{bal}
		}},
		{"$transfer_energy", func(env *bridge, binding *prototype.Binding) []interface{} {
			var amount *big.Int
			env.ParseArgs(&amount)

			transferData, err := energyTransferMethod.EncodeInput(env.To(), amount)
			if err != nil {
				panic(err)
			}
			// the msg caller send energy to this
			_, leftOverGas, vmerr := env.VM.Call(
				evm.AccountRef(env.Caller()),
				common.Address(Energy.Address),
				transferData,
				env.Contract.Gas,
				&big.Int{},
			)
			env.UseGas(env.Contract.Gas - leftOverGas)
			if vmerr != nil {
				env.Stop(vmerr)
			}
			return nil
		}},
		{"$move_energy_to", func(env *bridge, binding *prototype.Binding) []interface{} {
			var args struct {
				To     common.Address
				Amount *big.Int
			}
			env.ParseArgs(&args)
			env.UseGas(ethparams.SloadGas)
			// master or account itself is allowed
			env.Require(env.Caller() == binding.Master() || env.Caller() == env.To())

			transferData, err := energyTransferMethod.EncodeInput(args.To, args.Amount)
			if err != nil {
				panic(err)
			}
			_, leftOverGas, vmerr := env.VM.Call(
				env.Contract,
				common.Address(Energy.Address),
				transferData,
				env.Contract.Gas,
				&big.Int{},
			)
			env.UseGas(env.Contract.Gas - leftOverGas)
			if vmerr != nil {
				env.Stop(vmerr)
			}
			return nil
		}},
		{"$is_user", func(env *bridge, binding *prototype.Binding) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)
			env.UseGas(ethparams.SloadGas)
			isUser := binding.IsUser(thor.Address(addr))
			return []interface{}{isUser}
		}},
		{"$user_credit", func(env *bridge, binding *prototype.Binding) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)
			env.UseGas(ethparams.SloadGas)
			credit := binding.UserCredit(thor.Address(addr), env.BlockTime())
			return []interface{}{credit}
		}},
		{"$add_user", func(env *bridge, binding *prototype.Binding) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)
			env.UseGas(ethparams.SloadGas)

			// master or account itself is allowed
			env.Require(env.Caller() == binding.Master() || env.Caller() == env.To())
			env.UseGas(ethparams.SloadGas)
			env.Require(binding.AddUser(thor.Address(addr), env.BlockTime()))

			env.UseGas(ethparams.SloadGas)
			env.UseGas(ethparams.SstoreSetGas)
			env.Log(addRemoveUserEvent, []thor.Bytes32{thor.BytesToBytes32(addr[:])}, true)

			return nil
		}},
		{"$remove_user", func(env *bridge, binding *prototype.Binding) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)

			env.UseGas(ethparams.SloadGas)
			// master or account itself is allowed
			env.Require(env.Caller() == binding.Master() || env.Caller() == env.To())

			env.UseGas(ethparams.SloadGas)
			env.Require(binding.RemoveUser(thor.Address(addr)))

			env.UseGas(ethparams.SstoreClearGas)
			env.Log(addRemoveUserEvent, []thor.Bytes32{thor.BytesToBytes32(addr[:])}, false)

			return nil
		}},
		{"$user_plan", func(env *bridge, binding *prototype.Binding) []interface{} {
			env.UseGas(ethparams.SloadGas)
			credit, rate := binding.UserPlan()
			return []interface{}{credit, rate}
		}},
		{"$set_user_plan", func(env *bridge, binding *prototype.Binding) []interface{} {
			var args struct {
				Credit       *big.Int
				RecoveryRate *big.Int
			}
			env.ParseArgs(&args)

			env.UseGas(ethparams.SloadGas)
			// master or account itself is allowed
			env.Require(env.Caller() == binding.Master() || env.Caller() == env.To())

			env.UseGas(ethparams.SstoreSetGas)
			binding.SetUserPlan(args.Credit, args.RecoveryRate)
			env.Log(setUserPlanEvent, nil, args.Credit, args.RecoveryRate)
			return nil
		}},
		{"$sponsor", func(env *bridge, binding *prototype.Binding) []interface{} {
			var yesOrNo bool
			env.ParseArgs(&yesOrNo)
			env.UseGas(ethparams.SloadGas)
			env.Require(binding.Sponsor(env.Caller(), yesOrNo))

			if yesOrNo {
				env.UseGas(ethparams.SstoreSetGas)
			} else {
				env.UseGas(ethparams.SstoreClearGas)
			}
			env.Log(sponsorEvent, []thor.Bytes32{thor.BytesToBytes32(env.Caller().Bytes())}, yesOrNo)
			return nil
		}},
		{"$is_sponsor", func(env *bridge, binding *prototype.Binding) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)
			env.UseGas(ethparams.SloadGas)
			b := binding.IsSponsor(thor.Address(addr))
			return []interface{}{b}
		}},
		{"$select_sponsor", func(env *bridge, binding *prototype.Binding) []interface{} {
			var addr common.Address
			env.ParseArgs(&addr)
			env.UseGas(ethparams.SloadGas)
			// master or account itself is allowed
			env.Require(env.Caller() == binding.Master() || env.Caller() == env.To())
			env.UseGas(ethparams.SloadGas)
			env.Require(binding.SelectSponsor(thor.Address(addr)))

			env.UseGas(ethparams.SstoreResetGas)
			env.Log(selectSponsorEvent, []thor.Bytes32{thor.BytesToBytes32(addr[:])})
			return nil
		}},
		{"$current_sponsor", func(env *bridge, binding *prototype.Binding) []interface{} {
			env.UseGas(ethparams.SloadGas)
			addr := binding.CurrentSponsor()
			return []interface{}{addr}
		}},
	}

	for _, def := range defines {
		def := def // make a copy since it's used in closure
		if method, found := nativeABI.MethodByName(def.name); found {
			prototypeMethods[method.ID()] = &nativeMethod{
				ABI: method,
				Run: func(env *bridge) []interface{} {
					return def.run(env, Prototype.Native(env.State).Bind(env.To()))
				},
			}
		} else {
			panic("method not found: " + def.name)
		}
	}

}

const (
	blake2b256WordGas uint64 = 3
	blake2b256Gas     uint64 = 15
)

func initExtensionMethods() {
	defines := []struct {
		name string
		run  func(env *bridge) []interface{}
	}{
		{"native_blake2b256", func(env *bridge) []interface{} {
			var data []byte
			env.ParseArgs(&data)
			env.UseGas(uint64(len(data)+31)/32*blake2b256WordGas + blake2b256Gas)
			output := Extension.Native(env.State).Blake2b256(data)
			return []interface{}{output}
		}},
	}

	nativeABI := Extension.NativeABI()
	for _, def := range defines {
		if method, found := nativeABI.MethodByName(def.name); found {
			privateMethods[addressAndMethodID{Extension.Address, method.ID()}] = &nativeMethod{
				ABI: method,
				Run: def.run,
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
	initPrototypeInterfaceMethods()
	initExtensionMethods()
}

// HandleNativeCall entry of native methods implementaion.
func HandleNativeCall(
	state *state.State,
	vm *evm.EVM,
	contract *evm.Contract,
	readonly bool,
) func() ([]byte, error) {

	methodID, err := abi.ExtractMethodID(contract.Input)
	if err != nil {
		return nil
	}

	var method *nativeMethod
	if contract.Address() == contract.Caller() {
		// private methods require caller == to
		method = privateMethods[addressAndMethodID{thor.Address(contract.Address()), methodID}]
	}

	if method == nil {
		method = prototypeMethods[methodID]
	}

	if method == nil {
		return nil
	}

	if readonly && !method.ABI.Const() {
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
	return newBridge(method, state, vm, contract).Call
}
