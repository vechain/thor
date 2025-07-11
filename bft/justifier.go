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
	parentQuality uint32
	checkpoint    uint32
	threshold     *big.Int // we need to represent uint256
	isPoS         bool     // true for PoS, false for PoA

	votes        map[thor.Address]bool
	voterWeights map[thor.Address]*big.Int
	comVotes     uint64
	comWeight    *big.Int
}

func newJustifier(parentQuality, checkpoint uint32, threshold *big.Int, isPoS bool) *justifier {
	return &justifier{
		votes:         make(map[thor.Address]bool),
		voterWeights:  make(map[thor.Address]*big.Int),
		parentQuality: parentQuality,
		checkpoint:    checkpoint,
		threshold:     threshold,
		isPoS:         isPoS,
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

	if blockNum >= engine.forkConfig.HAYABUSA {
		totalWeight, err := engine.getTotalWeight(sum)
		if err != nil {
			return nil, err
		}
		threshold := new(big.Int).Mul(totalWeight, big.NewInt(2))
		threshold.Div(threshold, big.NewInt(3))
		return newJustifier(parentQuality, checkpoint, threshold, true), nil
	} else {
		mbp, err := engine.getMaxBlockProposers(sum)
		if err != nil {
			return nil, err
		}
		threshold := mbp * 2 / 3
		return newJustifier(parentQuality, checkpoint, new(big.Int).SetUint64(threshold), false), nil
	}
}

func (js *justifier) AddBlock(signer thor.Address, isCOM bool, weight *big.Int) {
	if !js.isPoS {
		// Pre-HAYABUSA
		if prev, ok := js.votes[signer]; !ok {
			js.votes[signer] = isCOM
			if isCOM {
				js.comVotes++
			}
		} else if prev != isCOM {
			// if one votes both COM and non-COM in one round, count as non-COM
			js.votes[signer] = false
			if prev {
				js.comVotes--
			}
		}
	} else {
		// HAYABUSA
		if prev, ok := js.voterWeights[signer]; !ok {
			js.voterWeights[signer] = new(big.Int).Set(weight)
			if isCOM {
				js.comWeight.Add(js.comWeight, weight)
			}
		} else if !isCOM {
			js.comWeight.Sub(js.comWeight, prev)
		}
	}
}

func (js *justifier) Summarize() *bftState {
	var justified, committed bool

	// Pre-HAYABUSA
	if !js.isPoS {
		justified = uint64(len(js.votes)) > js.threshold.Uint64()
		committed = js.comVotes > js.threshold.Uint64()
	} else {
		// HAYABUSA
		totalVoterWeight := big.NewInt(0)
		for _, weight := range js.voterWeights {
			totalVoterWeight.Add(totalVoterWeight, weight)
		}
		justified = totalVoterWeight.Cmp(js.threshold) > 0
		committed = js.comWeight.Cmp(js.threshold) > 0
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
