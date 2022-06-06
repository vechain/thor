// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// bftState is the state summary of a bft round for a given block.
type bftState struct {
	Quality   uint32 // accumulated justified block count
	Justified bool
	CommitAt  *thor.Bytes32 // block reaches committed in this round
}

// voteSet tracks vote in a bft round.
type voteSet struct {
	parentQuality uint32
	checkpoint    uint32
	threshold     uint64

	votes    map[thor.Address]block.Vote
	comVotes uint64
	commitAt *thor.Bytes32
}

func newVoteSet(engine *BFTEngine, parentID thor.Bytes32) (*voteSet, error) {
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

	return &voteSet{
		votes:         make(map[thor.Address]block.Vote),
		parentQuality: parentQuality,
		checkpoint:    checkpoint,
		threshold:     threshold,
	}, nil
}

func (vs *voteSet) isCommitted() bool {
	return vs.commitAt != nil
}

// addVote adds a new vote to the set.
func (vs *voteSet) addVote(signer thor.Address, vote block.Vote, blockID thor.Bytes32) {
	if vs.isCommitted() {
		return
	}

	if prev, ok := vs.votes[signer]; !ok {
		vs.votes[signer] = vote
		if vote == block.COM {
			vs.comVotes++
		}
	} else if prev == block.WIT && vote == block.COM {
		vs.votes[signer] = vote
		vs.comVotes++
	}

	if vs.commitAt == nil && vs.comVotes > vs.threshold {
		vs.commitAt = &blockID
	}
}

func (vs *voteSet) getState() *bftState {
	justified := false
	quality := vs.parentQuality
	if len(vs.votes) > int(vs.threshold) {
		justified = true
		quality = quality + 1
	}

	return &bftState{
		Quality:   quality,
		Justified: justified,
		CommitAt:  vs.commitAt,
	}
}
