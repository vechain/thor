// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
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

// newJustifier creates a justifier for the block described by sum, which may not yet
// be in the repo (during bft.Select) — sum.Conflicts lets us address its state.
//
// Two state snapshots:
//   - qualitySum (checkpoint-1): previous round's last block; source of prev quality
//     and the PoA threshold (max block proposers).
//   - thresholdSum (checkpoint): post-housekeep state, source of PoS total weight.
//     For the checkpoint block itself (not yet in repo) the root comes from sum.
func (engine *Engine) newJustifier(sum *chain.BlockSummary) (*justifier, error) {
	header := sum.Header
	blockNum := header.Number()
	parentID := header.ParentID()
	checkpoint := getCheckPoint(blockNum)

	var lastOfParentRound uint32
	if checkpoint > 0 {
		lastOfParentRound = checkpoint - 1
	}

	qualitySum, err := engine.repo.NewChain(parentID).GetBlockSummary(lastOfParentRound)
	if err != nil {
		return nil, err
	}

	var parentQuality uint32 // quality of last round
	if absRound := blockNum/thor.EpochLength() - engine.forkConfig.FINALITY/thor.EpochLength(); absRound != 0 {
		parentQuality, err = engine.getQuality(qualitySum.Header.ID())
		if err != nil {
			return nil, err
		}
	}

	// posActive and threshold need the checkpoint's post-housekeep state: sum itself
	// when blockNum == checkpoint, else fetched from repo.
	thresholdSum := sum
	if blockNum != checkpoint {
		thresholdSum, err = engine.repo.NewChain(parentID).GetBlockSummary(checkpoint)
		if err != nil {
			return nil, err
		}
	}

	thresholdState := engine.stater.NewState(thresholdSum.Root())
	posActive, err := builtin.Staker.Native(thresholdState).IsPoSActive()
	if err != nil {
		return nil, err
	}

	if posActive {
		totalWeight, err := engine.getTotalWeight(thresholdSum)
		if err != nil {
			return nil, err
		}
		return newJustifier(parentQuality, checkpoint, 0, totalWeight*2/3), nil
	}

	// PoA threshold: max block proposers is housekeep-independent, read from qualitySum.
	mbp, err := engine.getMaxBlockProposers(qualitySum)
	if err != nil {
		return nil, err
	}
	return newJustifier(parentQuality, checkpoint, mbp*2/3, 0), nil
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

// Summarize summarizes the state of vote set.
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
