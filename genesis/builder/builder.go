package builder

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/bn"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/processor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

const (
	// gas limit when execute vm call in build steps.
	execGasLimit = 100000000
)

// Builder helper to build genesis block.
type Builder struct {
	timestamp uint64
	gasLimit  bn.Int

	allocs []alloc
	calls  []call
}

type alloc struct {
	address          acc.Address
	balance          bn.Int
	runtimeBytecodes []byte
}

type call struct {
	address acc.Address
	data    []byte
}

// Timestamp set timestamp.
func (b *Builder) Timestamp(t uint64) *Builder {
	b.timestamp = t
	return b
}

// GasLimit set gas limit.
func (b *Builder) GasLimit(gl *big.Int) *Builder {
	b.gasLimit = gl
	return b
}

// Alloc alloc an account with balance and runtime bytecodes.
func (b *Builder) Alloc(addr acc.Address, balance *big.Int, runtimeBytecodes []byte) *Builder {
	b.allocs = append(b.allocs, alloc{
		addr,
		balance,
		runtimeBytecodes,
	})
	return b
}

// Call call the pre alloced contract(account with runtime bytecodes).
func (b *Builder) Call(addr acc.Address, data []byte) *Builder {
	b.calls = append(b.calls, call{
		addr,
		data,
	})
	return b
}

func (b *Builder) newVM(state State, origin acc.Address) *vm.VM {
	return vm.New(vm.Context{
		Origin:      origin,
		Beneficiary: acc.Address{},
		BlockNumber: big.NewInt(0),
		Time:        new(big.Int).SetUint64(b.timestamp),
		GasLimit:    big.NewInt(execGasLimit),
		GasPrice:    new(big.Int),
		TxHash:      cry.Hash{},
		ClauseIndex: 0,
		GetHash: func(uint64) cry.Hash {
			return cry.Hash{}
		},
	}, state, vm.Config{})
}

// Build build genesis block according to presets.
func (b *Builder) Build(state State, god acc.Address) (*block.Block, error) {

	// alloc all requested accounts
	for _, alloc := range b.allocs {
		state.SetBalance(alloc.address, alloc.balance)
		if len(alloc.runtimeBytecodes) > 0 {
			state.SetCode(alloc.address, alloc.runtimeBytecodes)
			continue
		}
	}

	// execute all calls
	for _, call := range b.calls {
		vm := b.newVM(state, god)
		output, _, _ := vm.Call(
			god,
			call.address,
			call.data,
			execGasLimit,
			new(big.Int),
		)
		if output.VMErr != nil {
			return nil, errors.Wrap(output.VMErr, "build genesis (vm error)")
		}
		if err := (*processor.VMOutput)(output).ApplyState(state); err != nil {
			return nil, errors.Wrap(err, "build genesis")
		}
	}

	stateRoot := state.Commit()
	if err := state.Error(); err != nil {
		return nil, errors.Wrap(err, "build genesis")
	}

	return new(block.Builder).
			Timestamp(b.timestamp).
			GasLimit(b.gasLimit).
			StateRoot(stateRoot).
			ReceiptsRoot(tx.EmptyRoot).
			Build(),
		nil
}
