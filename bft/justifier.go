// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
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
	threshold     uint64

	votes    map[thor.Address]block.Vote
	comVotes uint64
}

func newJustifier(engine *BFTEngine, parentID thor.Bytes32) (*justifier, error) {
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
	mbp, err := engine.getMaxBlockProposers(sum)
	if err != nil {
		return nil, err
	}
	threshold := mbp * 2 / 3

	var parentQuality uint32 // quality of last round
	if absRound := blockNum/thor.CheckpointInterval - engine.forkConfig.FINALITY/thor.CheckpointInterval; absRound == 0 {
		parentQuality = 0
	} else {
		var err error
		parentQuality, err = engine.getQuality(sum.Header.ID())
		if err != nil {
			return nil, err
		}
	}

	return &justifier{
		votes:         make(map[thor.Address]block.Vote),
		parentQuality: parentQuality,
		checkpoint:    checkpoint,
		threshold:     threshold,
	}, nil
}

// AddBlock adds a new block to the set.
func (js *justifier) AddBlock(blockID thor.Bytes32, signer thor.Address, vote block.Vote) {
	if prev, ok := js.votes[signer]; !ok {
		js.votes[signer] = vote
		if vote == block.COM {
			js.comVotes++
		}
	} else if prev != vote {
		// if one votes both WIT and COM in one round, count as WIT
		js.votes[signer] = block.WIT
		if prev == block.COM {
			js.comVotes--
		}
	}
}

// Summarize summarizes the state of vote set.
func (js *justifier) Summarize() *bftState {
	justified := len(js.votes) > int(js.threshold)

	var quality uint32
	if justified {
		quality = js.parentQuality + 1
	} else {
		quality = js.parentQuality
	}

	return &bftState{
		Quality:   quality,
		Justified: justified,
		Committed: js.comVotes > js.threshold,
	}
}
