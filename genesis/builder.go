// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"math"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

// Builder helper to build genesis block.
type Builder struct {
	timestamp uint64
	gasLimit  uint64

	stateProcs []func(state *state.State) error
	calls      []call
	extraData  [28]byte

	forkConfig thor.ForkConfig
}

type call struct {
	clause *tx.Clause
	caller thor.Address
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

// State add a state process !!!touch accounts's energy is mandatory if you touch its balance
func (b *Builder) State(proc func(state *state.State) error) *Builder {
	b.stateProcs = append(b.stateProcs, proc)
	return b
}

// Call add a contract call.
func (b *Builder) Call(clause *tx.Clause, caller thor.Address) *Builder {
	b.calls = append(b.calls, call{clause, caller})
	return b
}

// ExtraData set extra data, which will be put into last 28 bytes of genesis parent id.
func (b *Builder) ExtraData(data [28]byte) *Builder {
	b.extraData = data
	return b
}

// ForkConfig set fork config.
func (b *Builder) ForkConfig(fc thor.ForkConfig) *Builder {
	b.forkConfig = fc
	return b
}

// ComputeID compute genesis ID.
func (b *Builder) ComputeID() (thor.Bytes32, error) {
	db := muxdb.NewMem()

	blk, _, _, err := b.Build(state.NewStater(db))
	if err != nil {
		return thor.Bytes32{}, err
	}
	return blk.Header().ID(), nil
}

// Build build genesis block according to presets.
func (b *Builder) Build(stater *state.Stater) (blk *block.Block, events tx.Events, transfers tx.Transfers, err error) {
	state := stater.NewState(thor.Bytes32{}, 0, 0, 0)

	for _, proc := range b.stateProcs {
		if err := proc(state); err != nil {
			return nil, nil, nil, errors.Wrap(err, "state process")
		}
	}

	rt := runtime.New(nil, state, &xenv.BlockContext{
		Time:     b.timestamp,
		GasLimit: b.gasLimit,
	}, b.forkConfig)

	for _, call := range b.calls {
		exec, _ := rt.PrepareClause(call.clause, 0, math.MaxUint64, &xenv.TransactionContext{
			Origin: call.caller,
		})
		out, _, err := exec()
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "call")
		}
		if out.VMErr != nil {
			return nil, nil, nil, errors.Wrap(out.VMErr, "vm")
		}
		events = append(events, out.Events...)
		transfers = append(transfers, out.Transfers...)
	}

	stage, err := state.Stage(0, 0)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "stage")
	}
	stateRoot, err := stage.Commit()
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "commit state")
	}

	parentID := thor.Bytes32{0xff, 0xff, 0xff, 0xff} //so, genesis number is 0
	copy(parentID[4:], b.extraData[:])

	return new(block.Builder).
		ParentID(parentID).
		Timestamp(b.timestamp).
		GasLimit(b.gasLimit).
		StateRoot(stateRoot).
		ReceiptsRoot(tx.Transactions(nil).RootHash()).
		Build(), events, transfers, nil
}
