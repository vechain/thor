package consensus

import (
	"bytes"
	"math"
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
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

// MeasureTxDelay 返回该交易的延迟时间.
func MeasureTxDelay(blockRef tx.BlockRef, parent *block.Header, chain *chain.Chain) (uint64, error) {
	parentNum := parent.Number()
	refNum := blockRef.Number()
	if refNum > parentNum {
		return math.MaxUint64, nil
	}

	if uint64(parentNum-refNum)*thor.BlockInterval > thor.MaxTxWorkDelay {
		return math.MaxUint64, nil
	}

	header := parent
	var err error
	for refNum <= header.Number() {
		if header.Number() == refNum {
			if bytes.HasPrefix(header.ID().Bytes(), blockRef[:]) {
				return parent.Timestamp() - header.Timestamp(), nil
			}
			break
		}

		header, err = chain.GetBlockHeader(header.ParentID())
		if err != nil {
			return 0, err
		}
	}
	return math.MaxUint64, nil
}

// CalcReward 返回交易的 reward.
func CalcReward(tx *tx.Transaction, gasUsed uint64, ratio *big.Int, timestamp uint64, delay uint64) *big.Int {

	x := new(big.Int).SetUint64(tx.Gas())
	x.Mul(x, tx.GasPrice()) // tx defined energy (TDE)

	var y *big.Int
	if delay > thor.MaxTxWorkDelay {
		y = &big.Int{}
	} else {
		// work produced energy (WPE)
		y = thor.ProvedWork.ToEnergy(tx.ProvedWork(), timestamp)
	}

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
