package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/state"
)

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
