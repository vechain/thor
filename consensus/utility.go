package consensus

import (
	"math"
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

func handleClause(rt *runtime.Runtime, clause *tx.Clause) *vm.Output {
	return rt.Call(clause, 0, math.MaxUint64, *clause.To(), &big.Int{}, thor.Hash{})
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

func getRewardPercentage(rt *runtime.Runtime) (uint64, error) {
	output := handleClause(rt,
		contracts.Params.PackGet(contracts.ParamRewardRatio))

	if output.VMErr != nil {
		return 0, errors.Wrap(output.VMErr, "reward percentage")
	}

	return contracts.Params.UnpackGet(output.Value).Uint64() / 1E18, nil
}
