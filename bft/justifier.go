// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"math/big"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

// bftState is the summary of a bft round for a given head.
type bftState struct {
	Quality   uint32 // accumulated justified block count
	Justified bool
	Committed bool
}

// justifier tracks all block vote in one bft round and justify the round.
type justifier struct {
	parentQuality   uint32
	checkpoint      uint32
	votesThreshold  uint64
	weightThreshold *big.Int // we need to represent uint256

	votes        map[thor.Address]bool
	voterWeights map[thor.Address]*big.Int
	comVotes     uint64
	comWeight    *big.Int
}

func newJustifier(parentQuality, checkpoint uint32, votesThreshold uint64, weightThreshold *big.Int) *justifier {
	return &justifier{
		votes:         make(map[thor.Address]bool),
		voterWeights:  make(map[thor.Address]*big.Int),
		parentQuality: parentQuality,
		checkpoint:    checkpoint,
		votesThreshold: votesThreshold,
		weightThreshold: weightThreshold,
		comWeight:     big.NewInt(0),
	}
}

func (engine *Engine) newJustifier(parentID thor.Bytes32) (*justifier, error) {
	blockNum := block.Number(parentID) + 1

	var lastOfParentRound uint32
	checkpoint := getCheckPoint(blockNum)
	if checkpoint > 0 {
		lastOfParentRound = checkpoint - 1
	} else {
		lastOfParentRound = 0
	}

	sum, err := engine.repo.NewChain(parentID).GetBlockSummary(lastOfParentRound)
	if err != nil {
		return nil, err
	}

	var parentQuality uint32
	if absRound := blockNum/thor.CheckpointInterval - engine.forkConfig.FINALITY/thor.CheckpointInterval; absRound == 0 {
		parentQuality = 0
	} else {
		var err error
		parentQuality, err = engine.getQuality(sum.Header.ID())
		if err != nil {
			return nil, err
		}
	}

	if lastOfParentRound >= engine.forkConfig.HAYABUSA+engine.forkConfig.HAYABUSA_TP {
		totalWeight, err := engine.getTotalWeight(sum)
		if err != nil {
			return nil, err
		}
		weightThreshold := new(big.Int).Mul(totalWeight, big.NewInt(2))
		weightThreshold.Div(weightThreshold, big.NewInt(3))
		return newJustifier(parentQuality, checkpoint, 0, weightThreshold), nil
	} else {
		mbp, err := engine.getMaxBlockProposers(sum)
		if err != nil {
			return nil, err
		}
		votesThreshold := uint64(mbp * 2 / 3)
		return newJustifier(parentQuality, checkpoint, votesThreshold, nil), nil
	}
}

func (js *justifier) AddBlock(signer thor.Address, isCOM bool, weight *big.Int) {
	// Boolean count is required due to COM regardless of PoS or PoA
	if prev, ok := js.votes[signer]; !ok {
		js.votes[signer] = isCOM
		if isCOM {
			js.comVotes++
			if weight != nil {
				js.voterWeights[signer] = new(big.Int).Set(weight)
				js.comWeight.Add(js.comWeight, weight)
			}
		}
	} else if prev != isCOM {
		// if one votes both COM and non-COM in one round, count as non-COM
		js.votes[signer] = false
		if prev {
			js.comVotes--
			if prevWeight, ok := js.voterWeights[signer]; ok {
				js.comWeight.Sub(js.comWeight, prevWeight)
			}
		}
	}
}

func (js *justifier) Summarize() *bftState {
	var justified, committed bool

	// Pre-HAYABUSA
	if js.weightThreshold == nil {
		justified = uint64(len(js.votes)) > js.votesThreshold
		committed = js.comVotes > js.votesThreshold
	} else {
		totalVoterWeight := big.NewInt(0)
		for _, weight := range js.voterWeights {
			totalVoterWeight.Add(totalVoterWeight, weight)
		}
		justified = totalVoterWeight.Cmp(js.weightThreshold) > 0
		committed = js.comWeight.Cmp(js.weightThreshold) > 0
	}

	var quality uint32
	if justified {
		quality = js.parentQuality + 1
	} else {
		quality = js.parentQuality
	}

	return &bftState{
		Quality:   quality,
		Justified: justified,
		Committed: committed,
	}
}
