package genesis

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
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

// Build build genesis block according to presets.
func (b *Builder) Build(stateCreator *state.Creator) (blk *block.Block, err error) {
	state, err := stateCreator.NewState(thor.Hash{})
	if err != nil {
		return nil, err
	}

	for _, proc := range b.stateProcs {
		if err := proc(state); err != nil {
			return nil, errors.Wrap(err, "state process")
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
