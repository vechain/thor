package consensus

import (
	"bytes"
	"math"
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
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

// MeasureTxDelay 返回该交易的延迟时间.
func MeasureTxDelay(blockRef tx.BlockRef, parentBlockID thor.Hash, chain *chain.Chain) (uint32, error) {
	parentNum := block.Number(parentBlockID)
	refNum := blockRef.Number()
	if refNum > parentNum {
		return 0, errors.New("ref num > parent block num")
	}
	diff := parentNum - refNum
	if diff > thor.MaxTxWorkDelay {
		return math.MaxUint32, nil
	}
	blockID := parentBlockID
	for i := uint32(0); i < diff; i++ {
		header, err := chain.GetBlockHeader(blockID)
		if err != nil {
			return 0, err
		}
		blockID = header.ParentID()
	}
	if bytes.HasPrefix(blockID[:], blockRef[:]) {
		return diff, nil
	}
	return math.MaxUint32, nil
}

// CalcReward 返回交易的 reward.
func CalcReward(tx *tx.Transaction, gasUsed uint64, ratio *big.Int, blockNum uint32, delay uint32) *big.Int {
	x := new(big.Int).SetUint64(tx.Gas())
	x.Mul(x, tx.GasPrice()) // tx defined energy (TDE)

	// work produced energy (WPE)
	y := thor.ProvedWorkToEnergy(tx.ProvedWork(), blockNum, delay)

	// limit WPE to atmost TDE
	if y.Cmp(x) > 0 {
		x.Add(x, y)
	} else {
		x.Add(x, y)
	}

	// overall gas price
	y.Div(x, y.SetUint64(tx.Gas()))

	// overall consumed energy
	x.Mul(x.SetUint64(gasUsed), y)

	x.Mul(x, ratio)
	return x.Div(x, big.NewInt(1e18))
}
