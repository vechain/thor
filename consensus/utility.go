package consensus

import (
	"math"
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

func handleClause(rt *runtime.Runtime, to thor.Address, data []byte) *vm.Output {
	clause := &tx.Clause{
		To:   &to,
		Data: data}
	return rt.Execute(clause, 0, math.MaxUint64, to, &big.Int{}, thor.Hash{})
}

func checkState(state *state.State, header *block.Header) error {
	if stateRoot, err := state.Stage().Hash(); err == nil {
		if header.StateRoot() != stateRoot {
			return errStateRoot
		}
	} else {
		return err
	}
	return nil
}

func calcScore(proposers []schedule.Proposer, updates []schedule.Proposer) uint64 {
	witness := make(map[thor.Address]bool)

	for _, proposer := range proposers {
		if !proposer.IsAbsent() {
			witness[proposer.Address] = true
		}
	}

	for _, update := range updates {
		if update.IsAbsent() {
			delete(witness, update.Address)
		} else {
			witness[update.Address] = true
		}
	}

	return uint64(len(witness))
}
