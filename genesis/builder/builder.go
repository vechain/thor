package builder

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/bn"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
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
	b.gasLimit.SetBig(gl)
	return b
}

// Alloc alloc an account with balance and runtime bytecodes.
func (b *Builder) Alloc(addr acc.Address, balance *big.Int, runtimeBytecodes []byte) *Builder {
	b.allocs = append(b.allocs, alloc{
		addr,
		bn.FromBig(balance),
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

func (b *Builder) newRuntime(state *state.State, origin acc.Address) *runtime.Runtime {
	return runtime.New(
		state,
		&block.Header{},
		func(uint64) cry.Hash { return cry.Hash{} },
		vm.Config{},
	)
}

// Build build genesis block according to presets.
func (b *Builder) Build(state *state.State, god acc.Address) (*block.Block, error) {

	// alloc all requested accounts
	for _, alloc := range b.allocs {
		state.SetBalance(alloc.address, alloc.balance.ToBig())
		if len(alloc.runtimeBytecodes) > 0 {
			state.SetCode(alloc.address, alloc.runtimeBytecodes)
			continue
		}
	}

	// execute all calls
	for _, call := range b.calls {
		rt := b.newRuntime(state, god)
		output := rt.Exec(
			&tx.Clause{
				To:   &call.address,
				Data: call.data},
			0,
			execGasLimit,
			god,
			new(big.Int),
			cry.Hash{})
		if output.VMErr != nil {
			return nil, errors.Wrap(output.VMErr, "build genesis (vm error)")
		}
	}

	stage := state.Stage()
	stateRoot, err := stage.Commit()
	if err != nil {
		return nil, errors.Wrap(err, "commit state")
	}

	return new(block.Builder).
			Timestamp(b.timestamp).
			GasLimit(b.gasLimit.ToBig()).
			StateRoot(stateRoot).
			ReceiptsRoot(tx.EmptyRoot).
			Build(),
		nil
}
