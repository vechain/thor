package consensus

import (
	"math"
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

func handleClause(rt *runtime.Runtime, to thor.Address, data []byte) *vm.Output {
	return rt.Execute(tx.NewClause(&to).WithData(data), 0, math.MaxUint64, to, &big.Int{}, thor.Hash{})
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
	return uint64(len(
		getPresentProposers(
			getPresentProposers(
				make(map[thor.Address]bool),
				proposers),
			updates)))
}

func getPresentProposers(witness map[thor.Address]bool, proposers []schedule.Proposer) map[thor.Address]bool {
	length := len(proposers)
	if length == 0 {
		return witness
	}

	if proposers[0].IsAbsent() {
		delete(witness, proposers[0].Address)
	} else {
		witness[proposers[0].Address] = true
	}

	return getPresentProposers(witness, proposers[1:length])
}

func getRewardPercentage(rt *runtime.Runtime) (uint64, error) {
	output := handleClause(rt,
		contracts.Params.Address,
		contracts.Params.PackGet(contracts.ParamRewardPercentage))

	if output.VMErr != nil {
		return 0, errors.Wrap(output.VMErr, "reward percentage")
	}

	return contracts.Params.UnpackGet(output.Value).Uint64() / 100, nil
}
