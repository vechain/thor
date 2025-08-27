// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
)

// bftState is the summary of a bft round for a given head.
type bftState struct {
	Quality   uint32 // accumulated justified block count
	Justified bool
	Committed bool
}

type vote struct {
	isCOM  bool
	weight uint64
}

// justifier tracks all block vote in one bft round and justify the round.
type justifier struct {
	parentQuality   uint32
	checkpoint      uint32
	thresholdVotes  uint64
	thresholdWeight uint64

	votes           map[thor.Address]vote
	comVotes        uint64
	comWeight       uint64
	justifiedWeight uint64
}

func newJustifier(parentQuality, checkpoint uint32, thresholdVotes uint64, thresholdWeight uint64) *justifier {
	return &justifier{
		votes:           make(map[thor.Address]vote),
		parentQuality:   parentQuality,
		checkpoint:      checkpoint,
		thresholdVotes:  thresholdVotes,
		thresholdWeight: thresholdWeight,
		comWeight:       0,
		justifiedWeight: 0,
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

	var parentQuality uint32 // quality of last round
	if absRound := blockNum/thor.EpochLength() - engine.forkConfig.FINALITY/thor.EpochLength(); absRound == 0 {
		parentQuality = 0
	} else {
		var err error
		parentQuality, err = engine.getQuality(sum.Header.ID())
		if err != nil {
			return nil, err
		}
	}

	state := engine.stater.NewState(sum.Root())
	staker := builtin.Staker.Native(state)
	posActive, err := staker.IsPoSActive()
	if err != nil {
		return nil, err
	}

	if posActive {
		totalWeight, err := engine.getTotalWeight(sum)
		if err != nil {
			return nil, err
		}
		thresholdWeight := totalWeight*2/3 + 1
		return newJustifier(parentQuality, checkpoint, 0, thresholdWeight), nil
	} else {
		mbp, err := engine.getMaxBlockProposers(sum)
		if err != nil {
			return nil, err
		}
		thresholdVotes := mbp * 2 / 3
		return newJustifier(parentQuality, checkpoint, thresholdVotes, 0), nil
	}
}

// AddBlock adds a new block to the set.
func (js *justifier) AddBlock(signer thor.Address, isCOM bool, weight uint64) {
	if prev, ok := js.votes[signer]; !ok {
		js.votes[signer] = vote{isCOM: isCOM, weight: weight}
		if weight != 0 {
			js.justifiedWeight += weight
		}
		if isCOM {
			js.comVotes++
			if weight != 0 {
				js.comWeight += weight
			}
		}
	} else if prev.isCOM != isCOM {
		// if one votes both COM and non-COM in one round, count as non-COM
		js.votes[signer] = vote{isCOM: false, weight: prev.weight}

		if prev.isCOM {
			js.comVotes--
			if prev.weight != 0 {
				js.comWeight -= prev.weight
			}
		}
	}
}

func (js *justifier) Summarize() *bftState {
	var justified, committed bool

	// Pre-HAYABUSA
	if js.thresholdWeight == 0 {
		justified = uint64(len(js.votes)) > js.thresholdVotes
		committed = js.comVotes > js.thresholdVotes
	} else {
		justified = js.justifiedWeight > js.thresholdWeight
		committed = js.comWeight > js.thresholdWeight
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
