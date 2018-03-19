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
	chainTag  byte
	timestamp uint64
	gasLimit  uint64

	stateProcs []func(state *state.State) error
	calls      []call
}

type call struct {
	clause *tx.Clause
	caller thor.Address
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

// State add a state process
func (b *Builder) State(proc func(state *state.State) error) *Builder {
	b.stateProcs = append(b.stateProcs, proc)
	return b
}

// Call add a contrct call.
func (b *Builder) Call(clause *tx.Clause, caller thor.Address) *Builder {
	b.calls = append(b.calls, call{clause, caller})
	return b
}

// Build build genesis block according to presets.
func (b *Builder) Build(stateCreator *state.Creator) (blk *block.Block, logs []*tx.Log, err error) {
	state, err := stateCreator.NewState(thor.Hash{})
	if err != nil {
		return nil, nil, err
	}

	for _, proc := range b.stateProcs {
		if err := proc(state); err != nil {
			return nil, nil, errors.Wrap(err, "state process")
		}
	}

	rt := runtime.New(state, thor.Address{}, 0, b.timestamp, b.gasLimit, func(uint32) thor.Hash { return thor.Hash{} })

	for _, call := range b.calls {
		out := rt.Call(call.clause, 0, math.MaxUint64, call.caller, &big.Int{}, thor.Hash{})
		if out.VMErr != nil {
			return nil, nil, errors.Wrap(out.VMErr, "vm")
		}
		for _, log := range out.Logs {
			logs = append(logs, (*tx.Log)(log))
		}
	}

	stage := state.Stage()
	stateRoot, err := stage.Commit()
	if err != nil {
		return nil, nil, errors.Wrap(err, "commit state")
	}

	return new(block.Builder).
		ParentID(thor.BytesToHash([]byte{b.chainTag})).
		Timestamp(b.timestamp).
		GasLimit(b.gasLimit).
		StateRoot(stateRoot).
		ReceiptsRoot(tx.Transactions(nil).RootHash()).
		Build(), logs, nil
}
