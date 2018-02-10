package contracts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/vechain/thor/contracts/rabi"
	"github.com/vechain/thor/contracts/sslot"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm/evm"
)

// keys of governance params.
var (
	ParamRewardRatio  = thor.BytesToHash([]byte("reward-ratio")) // use 1e18 as denominator
	ParamBaseGasPrice = thor.BytesToHash([]byte("base-gas-price"))
)

// Params binder of `Params` contract.
var Params = func() *params {
	addr := thor.BytesToAddress([]byte("par"))
	abi := mustLoadABI("compiled/Params.abi")
	return &params{
		addr,
		abi,
		rabi.New(abi),
		sslot.NewMap(addr, 100),
	}
}()

type params struct {
	Address thor.Address
	ABI     *abi.ABI
	rabi    *rabi.ReversedABI
	data    *sslot.Map
}

// RuntimeBytecodes load runtime byte codes.
func (p *params) RuntimeBytecodes() []byte {
	return mustLoadHexData("compiled/Params.bin-runtime")
}

// NativeGet native way to get param.
func (p *params) Get(state *state.State, key thor.Hash) *big.Int {
	var v big.Int
	p.data.ForKey(key).LoadStructed(state, (*stgBigInt)(&v))
	return &v
}

// NativeSet native way to set param.
func (p *params) Set(state *state.State, key thor.Hash, value *big.Int) {
	p.data.ForKey(key).SaveStructed(state, (*stgBigInt)(value))
}

// HandleNative helper method to hook VM contract calls.
func (p *params) HandleNative(state *state.State, input []byte) func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
	name, err := p.rabi.NameOf(input)
	if err != nil {
		return nil
	}
	switch name {
	case "nativeGetVoting":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas) {
				return nil, evm.ErrOutOfGas
			}
			return p.rabi.PackOutput(name, Voting.Address)
		}
	case "nativeGet":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas) {
				return nil, evm.ErrOutOfGas
			}
			var key common.Hash
			if err := p.rabi.UnpackInput(&key, name, input); err != nil {
				return nil, err
			}
			return p.rabi.PackOutput(name, p.Get(state, thor.Hash(key)))
		}
	case "nativeSet":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if caller != p.Address {
				// only allow params' address to access this method
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreResetGas) {
				return nil, evm.ErrOutOfGas
			}
			var args struct {
				Key   common.Hash
				Value *big.Int
			}
			if err := p.rabi.UnpackInput(&args, name, input); err != nil {
				return nil, err
			}
			p.Set(state, thor.Hash(args.Key), args.Value)
			return nil, nil
		}
	}
	return nil
}
