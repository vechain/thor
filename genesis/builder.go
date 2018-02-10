package genesis

import (
	"math"
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Builder helper to build genesis block.
type Builder struct {
	chainTag  byite
	timestamp uint64
	gasLimit  uint64

	allocs []alloc
	calls  []*tx.Clause
}

type alloc struct {
	address          thor.Address
	balance          *big.Int
	runtimeBytecodes []byte
}

// ChainTag set chain tag.
func (b *Builder) ChainTag(tag byte) *Builder {
	b.chainTag = tag
	return b
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
func (b *Builder) Call(clause *tx.Clause) *Builder {
	b.calls = append(b.calls, clause)
	return b
}

// Build build genesis block according to presets.
func (b *Builder) Build(stateCreator *state.Creator) (blk *block.Block, err error) {
	state, err := stateCreator.NewState(thor.Hash{})
	if err != nil {
		return nil, err
	}

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
		thor.Address{}, 0, b.timestamp, b.gasLimit,
		func(uint32) thor.Hash { return thor.Hash{} })

	// execute all calls
	for _, call := range b.calls {
		output := rt.Call(
			call,
			0,
			math.MaxUint64,
			*call.To(),
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
			ParentID(thor.BytesToHash([]byte{b.chainTag})).
			Timestamp(b.timestamp).
			GasLimit(b.gasLimit).
			StateRoot(stateRoot).
			ReceiptsRoot(tx.Transactions(nil).RootHash()).
			Build(),
		nil
}
