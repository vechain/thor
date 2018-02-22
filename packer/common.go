package packer

import (
	"bytes"
	"math"
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	cs "github.com/vechain/thor/contracts"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func MeasureTxDelay(blockRef tx.BlockRef, parentBlockID thor.Hash, chain *chain.Chain) (uint32, error) {
	parentNum := block.Number(parentBlockID)
	refNum := blockRef.Number()
	if refNum > parentNum {
		return math.MaxUint32, nil
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

func Schedule(rt *runtime.Runtime, proposer thor.Address, now uint64) (
	uint64, // when
	uint64, // score
	error,
) {
	proposers := cs.Authority.All(rt.State())

	// calc the time when it's turn to produce block
	sched, err := poa.NewScheduler(proposer, proposers, rt.BlockNumber(), rt.BlockTime())
	if err != nil {
		return 0, 0, err
	}

	newBlockTime := sched.Schedule(now)

	updates, score := sched.Updates(newBlockTime)

	for _, u := range updates {
		cs.Authority.Update(rt.State(), u.Address, u.Status)
	}

	return newBlockTime, score, nil
}

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
