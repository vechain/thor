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
	CommitAt  *thor.Bytes32 // block reaches committed in this round
}

// justifier tracks all block vote in one bft round and justify the round.
type justifier struct {
	parentQuality uint32
	checkpoint    uint32
	threshold     uint64

	votes    map[thor.Address]block.Vote
	comVotes uint64
	commitAt *thor.Bytes32
}

func newJustifier(engine *BFTEngine, parentID thor.Bytes32) (*justifier, error) {
	var parentQuality uint32 // quality of last round

	blockNum := block.Number(parentID) + 1
	checkpoint := getCheckPoint(blockNum)
	absRound := blockNum/thor.CheckpointInterval - engine.forkConfig.FINALITY/thor.CheckpointInterval

	var lastOfParentRound uint32
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

	if absRound == 0 {
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

func (js *justifier) isCommitted() bool {
	return js.commitAt != nil
}

// AddBlock adds a new block to the set.
func (js *justifier) AddBlock(blockID thor.Bytes32, signer thor.Address, vote block.Vote) {
	if js.isCommitted() {
		return
	}

	if prev, ok := js.votes[signer]; !ok {
		js.votes[signer] = vote
		if vote == block.COM {
			js.comVotes++
		}
	} else if prev == block.WIT && vote == block.COM {
		js.votes[signer] = vote
		js.comVotes++
	}

	if js.commitAt == nil && js.comVotes > js.threshold {
		js.commitAt = &blockID
	}
}

// Summarize summarizes the state of vote set.
func (js *justifier) Summarize() *bftState {
	justified := false
	quality := js.parentQuality
	if len(js.votes) > int(js.threshold) {
		justified = true
		quality = quality + 1
	}

	return &bftState{
		Quality:   quality,
		Justified: justified,
		CommitAt:  js.commitAt,
	}
}
