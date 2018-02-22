package contracts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/vechain/thor/contracts/native"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
)

var nativeCalls = map[thor.Address]struct {
	*contract
	calls map[string]*native.Callable
}{
	Params.Address: {
		Params.contract,
		map[string]*native.Callable{
			"nativeGetExecutor": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					return []interface{}{Executor.Address}, nil
				}},
			"nativeGet": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var key common.Hash
					env.Args(&key)
					v := Params.Get(env.State, thor.Hash(key))
					return []interface{}{v}, nil
				}},
			"nativeSet": {
				Gas:            ethparams.SstoreResetGas,
				RequiredCaller: &Params.Address,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var args struct {
						Key   common.Hash
						Value *big.Int
					}
					env.Args(&args)
					Params.Set(env.State, thor.Hash(args.Key), args.Value)
					return nil, nil
				},
			}},
	},
	Authority.Address: {
		Authority.contract,
		map[string]*native.Callable{
			"nativeGetExecutor": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					return []interface{}{Executor.Address}, nil
				}},
			"nativeAdd": {
				Gas:            ethparams.SstoreSetGas,
				RequiredCaller: &Authority.Address,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var args struct {
						Addr     common.Address
						Identity common.Hash
					}
					env.Args(&args)
					ok := Authority.Add(env.State, thor.Address(args.Addr), thor.Hash(args.Identity))
					return []interface{}{ok}, nil
				}},
			"nativeRemove": {
				Gas:            ethparams.SstoreClearGas,
				RequiredCaller: &Authority.Address,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var addr common.Address
					env.Args(&addr)
					ok := Authority.Remove(env.State, thor.Address(addr))
					return []interface{}{ok}, nil
				}},
			"nativeStatus": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var addr common.Address
					env.Args(&addr)
					ok, identity, status := Authority.Status(env.State, thor.Address(addr))
					return []interface{}{ok, identity, status}, nil
				}},
			"nativeCount": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					count := Authority.Count(env.State)
					return []interface{}{count}, nil
				}},
		},
	},
	Energy.Address: {
		Energy.contract,
		map[string]*native.Callable{
			"nativeGetExecutor": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					return []interface{}{Executor.Address}, nil
				}},
			"nativeGetTotalSupply": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					return []interface{}{Energy.GetTotalSupply(env.State)}, nil
				}},
			"nativeGetBalance": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var addr common.Address
					env.Args(&addr)
					bal := Energy.GetBalance(env.State, env.VMContext.Time, thor.Address(addr))
					return []interface{}{bal}, nil
				}},
			"nativeSetBalance": {
				Gas:            ethparams.SstoreResetGas,
				RequiredCaller: &Energy.Address,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var args struct {
						Addr    common.Address
						Balance *big.Int
					}
					env.Args(&args)
					Energy.SetBalance(env.State, env.VMContext.Time, thor.Address(args.Addr), args.Balance)
					return nil, nil
				},
			},
			"nativeAdjustGrowthRate": {
				Gas:            ethparams.SstoreSetGas,
				RequiredCaller: &Energy.Address,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var rate *big.Int
					env.Args(&rate)
					Energy.AdjustGrowthRate(env.State, env.VMContext.Time, rate)
					return nil, nil
				},
			},
			"nativeSetSharing": {
				Gas:            ethparams.SstoreSetGas,
				RequiredCaller: &Energy.Address,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var args struct {
						From         common.Address
						To           common.Address
						Credit       *big.Int
						RecoveryRate *big.Int
						Expiration   uint64
					}
					env.Args(&args)
					Energy.SetSharing(env.State, env.VMContext.Time,
						thor.Address(args.From), thor.Address(args.To), args.Credit, args.RecoveryRate, args.Expiration)
					return nil, nil
				},
			},
			"nativeGetSharingRemained": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var args struct {
						From common.Address
						To   common.Address
					}
					env.Args(&args)
					remained := Energy.GetSharingRemained(env.State, env.VMContext.Time, thor.Address(args.From), thor.Address(args.To))
					return []interface{}{remained}, nil
				},
			},
			"nativeSetContractMaster": {
				Gas:            ethparams.SstoreResetGas,
				RequiredCaller: &Energy.Address,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var args struct {
						ContractAddr common.Address
						Master       common.Address
					}
					env.Args(&args)
					Energy.SetContractMaster(env.State, thor.Address(args.ContractAddr), thor.Address(args.Master))
					return nil, nil
				},
			},
			"nativeGetContractMaster": {
				Gas: ethparams.SloadGas,
				Proc: func(env *native.Env) ([]interface{}, error) {
					var contractAddr common.Address
					env.Args(&contractAddr)
					master := Energy.GetContractMaster(env.State, thor.Address(contractAddr))
					return []interface{}{master}, nil
				},
			},
		},
	},
}

func init() {
	for _, contract := range nativeCalls {
		for name, call := range contract.calls {
			call.MethodCodec = contract.ABI.MustForMethod(name)
		}
	}
}

func HandleNativeCall(state *state.State, vmCtx *vm.Context, to thor.Address, input []byte) func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
	contract, found := nativeCalls[to]
	if !found {
		return nil
	}

	name, err := contract.ABI.MethodName(input)
	if err != nil {
		return nil
	}

	if call, ok := contract.calls[name]; ok {
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			return call.Call(state, vmCtx, caller, useGas, input)
		}
	}
	return nil
}
