package contracts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/vechain/thor/contracts/rabi"
	"github.com/vechain/thor/contracts/sslot"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm/evm"
)

// keys of governance params.
const (
	ParamRewardRatio  = "reward-ratio" // use 1e18 as denominator
	ParamBaseGasPrice = "base-gas-price"
)

// Params binder of `Params` contract.
var Params = func() *params {
	addr := thor.BytesToAddress([]byte("par"))
	abi := mustLoadABI("compiled/Params.abi")
	return &params{
		addr,
		abi,
		rabi.New(abi),
		sslot.New(addr, 100),
	}
}()

type params struct {
	Address thor.Address
	abi     *abi.ABI
	rabi    *rabi.ReversedABI
	sslot   *sslot.StorageSlot
}

func (p *params) RuntimeBytecodes() []byte {
	return mustLoadHexData("compiled/Params.bin-runtime")
}

// PackInitialize packs input data of `Params._initialize` function.
func (p *params) PackInitialize() *tx.Clause {
	return tx.NewClause(&p.Address).
		WithData(mustPack(p.abi, "_initialize", Voting.Address))
}

// PackSet packs input data of `Params.set` function.
func (p *params) PackSet(key string, value *big.Int) *tx.Clause {
	return tx.NewClause(&p.Address).
		WithData(mustPack(p.abi, "set", key, value))
}

func (p *params) NativeGet(state *state.State, key string) *big.Int {
	var v big.Int
	p.sslot.Get(state, p.sslot.MapKey(key), (*stgBigInt)(&v))
	return &v
}

func (p *params) NativeSet(state *state.State, key string, value *big.Int) {
	p.sslot.Set(state, p.sslot.MapKey(key), (*stgBigInt)(value))
}

func (p *params) HandleNative(state *state.State, input []byte) func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
	name, err := p.rabi.NameOf(input)
	if err != nil {
		return nil
	}
	switch name {
	case "nativeGet":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas) {
				return nil, evm.ErrOutOfGas
			}
			var key string
			if err := p.rabi.UnpackInput(&key, name, input); err != nil {
				return nil, err
			}
			return p.rabi.PackOutput(name, p.NativeGet(state, key))
		}
	case "nativeSet":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if caller != p.Address {
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreResetGas) {
				return nil, evm.ErrOutOfGas
			}
			var args struct {
				Key   string
				Value *big.Int
			}
			if err := p.rabi.UnpackInput(&args, name, input); err != nil {
				return nil, err
			}
			p.NativeSet(state, args.Key, args.Value)
			return nil, nil
		}
	}
	return nil
}
