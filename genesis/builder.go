package genesis

import (
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	// gas limit when execute vm call in build steps.
	execGasLimit = 100000000
)

// Builder helper to build genesis block.
type Builder struct {
	timestamp uint64
	gasLimit  uint64

	allocs []alloc
	calls  []call
}

type alloc struct {
	address          thor.Address
	balance          *big.Int
	runtimeBytecodes []byte
}

type call struct {
	address thor.Address
	data    []byte
}

// Timestamp set timestamp.
func (b *Builder) Timestamp(t uint64) *Builder {
	b.timestamp = t
	return b
}

// GasLimit set gas limit.
func (b *Builder) GasLimit(limit uint64) *Builder {
	b.gasLimit = limit
	return b
}

// Alloc alloc an account with balance and runtime bytecodes.
func (b *Builder) Alloc(addr thor.Address, balance *big.Int, runtimeBytecodes []byte) *Builder {
	b.allocs = append(b.allocs, alloc{
		addr,
		balance,
		runtimeBytecodes,
	})
	return b
}

// Call call the pre alloced contract(account with runtime bytecodes).
func (b *Builder) Call(addr thor.Address, data []byte) *Builder {
	b.calls = append(b.calls, call{
		addr,
		data,
	})
	return b
}

// Build build genesis block according to presets.
func (b *Builder) Build(state *state.State) (blk *block.Block, err error) {

	checkpoint := state.NewCheckpoint()
	defer func() {
		if err != nil {
			state.RevertTo(checkpoint)
		}
	}()

	// alloc all requested accounts
	for _, alloc := range b.allocs {
		state.SetBalance(alloc.address, alloc.balance)
		if len(alloc.runtimeBytecodes) > 0 {
			state.SetCode(alloc.address, alloc.runtimeBytecodes)
			continue
		}
	}

	rt := runtime.New(
		state,
		&block.Header{},
		func(uint64) thor.Hash { return thor.Hash{} })

	// execute all calls
	for _, call := range b.calls {

		output := rt.Execute(
			&tx.Clause{
				To:    &call.address,
				Value: &big.Int{},
				Data:  call.data},
			0,
			execGasLimit,
			call.address,
			&big.Int{},
			thor.Hash{})

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
			GasLimit(b.gasLimit).
			StateRoot(stateRoot).
			ReceiptsRoot(tx.EmptyRoot).
			Build(),
		nil
}
