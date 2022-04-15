// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// voteSet tracks vote in a bft round
type voteSet struct {
	parentWeight uint32
	checkpoint   uint32
	threshold    uint64

	votes     map[thor.Address]bool
	comVotes  uint64
	justifyAt *thor.Bytes32 // block reached justified in this round
	commitAt  *thor.Bytes32 // block reached committed in this round
}

// bftState is the state summary of a bft round for a given block.
type bftState struct {
	Weight    uint32
	JustifyAt *thor.Bytes32
	CommitAt  *thor.Bytes32
}

func newVoteSet(engine *BFTEngine, parentID thor.Bytes32) (*voteSet, error) {
	var parentWeight uint32 // parent round bft weight

	blockNum := block.Number(parentID) + 1
	checkpoint := getCheckpoint(blockNum)
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
		parentWeight = 0
	} else {
		var err error
		parentWeight, err = engine.getWeight(sum.Header.ID())
		if err != nil {
			return nil, err
		}
	}

	return &voteSet{
		votes:        make(map[thor.Address]bool),
		parentWeight: parentWeight,
		checkpoint:   checkpoint,
		threshold:    threshold,
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

	isCom := vote == block.COM
	if votedCom, ok := vs.votes[signer]; !ok {
		vs.votes[signer] = isCom
		if isCom {
			vs.comVotes++
		}
	} else if !votedCom && isCom {
		vs.votes[signer] = true
		vs.comVotes++
	}

	if vs.justifyAt == nil && len(vs.votes) > int(vs.threshold) {
		vs.justifyAt = &blockID
	}

	if vs.commitAt == nil && vs.comVotes > vs.threshold {
		vs.commitAt = &blockID
	}
}

func (vs *voteSet) getState() *bftState {
	weight := vs.parentWeight
	if vs.justifyAt != nil {
		weight = weight + 1
	}

	return &bftState{
		Weight:    weight,
		JustifyAt: vs.justifyAt,
		CommitAt:  vs.commitAt,
	}
}
