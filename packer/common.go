package packer

import (
	"bytes"
	"math"
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

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

func Schedule(rt *runtime.Runtime, proposer thor.Address, now uint64) (
	uint64, // when
	uint64, // score
	error,
) {
	// invoke `Authority.proposers()` to get current proposers whitelist
	out := rt.StaticCall(
		contracts.Authority.PackProposers(),
		0, math.MaxUint64, thor.Address{}, &big.Int{}, thor.Hash{})

	if out.VMErr != nil {
		return 0, 0, errors.Wrap(out.VMErr, "vm")
	}

	proposers := contracts.Authority.UnpackProposers(out.Value)

	// calc the time when it's turn to produce block
	targetTime, updates, err := poa.NewScheduler(proposers, rt.BlockNumber(), rt.BlockTime()).
		Schedule(proposer, now)

	if err != nil {
		return 0, 0, err
	}

	// update proposers' status
	out = rt.Call(
		contracts.Authority.PackUpdate(updates),
		0, math.MaxUint64, contracts.Authority.Address, &big.Int{}, thor.Hash{})

	if out.VMErr != nil {
		return 0, 0, errors.Wrap(out.VMErr, "vm")
	}
	return targetTime, poa.CalculateScore(proposers, updates), nil
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
