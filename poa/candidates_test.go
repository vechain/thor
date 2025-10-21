// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func generateCandidateList(candidateCount int) []*authority.Candidate {
	candidateList := make([]*authority.Candidate, 0, candidateCount)
	for range candidateCount {
		var nodeMaster, endorsor thor.Address
		var identity thor.Bytes32

		rand.Read(nodeMaster[:])
		rand.Read(endorsor[:])
		rand.Read(identity[:])

		candidateList = append(candidateList, &authority.Candidate{
			NodeMaster: nodeMaster,
			Endorsor:   endorsor,
			Identity:   identity,
			Active:     true,
		})
	}

	return candidateList
}

func TestNewCandidates(t *testing.T) {
	candidateList := generateCandidateList(5)

	// Call NewCandidates with the mock data
	candidates := NewCandidates(candidateList)

	// Check if the candidates object is correctly initialized
	assert.NotNil(t, candidates)
	assert.Equal(t, len(candidateList), len(candidates.list))

	// Check the internal maps (masters and endorsors)
	for _, candidate := range candidateList {
		_, exists := candidates.masters[candidate.NodeMaster]
		assert.True(t, exists, "NodeMaster should exist in masters map")

		_, exists = candidates.endorsors[candidate.Endorsor]
		assert.True(t, exists, "Endorsor should exist in endorsors map")

		assert.Nil(t, candidates.satisfied, "Satisfied must be nil")

		assert.False(t, candidates.referenced, "Referenced must be false")
	}
}

func TestCopy(t *testing.T) {
	// Create a mock candidate
	var nodeMaster, endorsor thor.Address
	var identity thor.Bytes32

	rand.Read(nodeMaster[:])
	rand.Read(endorsor[:])
	rand.Read(identity[:])

	candidate := &authority.Candidate{
		NodeMaster: nodeMaster,
		Endorsor:   endorsor,
		Identity:   identity,
		Active:     true,
	}

	originalCandidates := NewCandidates([]*authority.Candidate{candidate})

	// Call the Copy method
	copiedCandidates := originalCandidates.Copy()

	// Check that the original and the copy are not the same instance
	assert.NotSame(t, originalCandidates, copiedCandidates, "Original and copied instances should not be the same")

	// Check that the internal state of the original and the copy are equivalent
	assert.Equal(t, originalCandidates.list, copiedCandidates.list, "The candidate lists of the original and the copy should be equal")
	assert.Equal(t, originalCandidates.masters, copiedCandidates.masters, "The masters maps of the original and the copy should be equal")
	assert.Equal(t, originalCandidates.endorsors, copiedCandidates.endorsors, "The endorsors maps of the original and the copy should be equal")
	assert.Equal(t, originalCandidates.satisfied, copiedCandidates.satisfied, "The satisfied list of the original and the copy should be equal")
	assert.Equal(t, originalCandidates.referenced, copiedCandidates.referenced, "The referenced state of the original and the copy should be equal")

	// Modify the copy and check that the original remains unchanged
	var newEndorsor thor.Address
	rand.Read(newEndorsor[:])
	copiedCandidates.endorsors[newEndorsor] = true

	_, existsInOriginal := originalCandidates.endorsors[newEndorsor]
	assert.True(t, existsInOriginal, "Modifying the copy should affect the original")
	assert.True(t, originalCandidates.referenced, "After copy, referenced must be true")
}

func TestPick(t *testing.T) {
	state := state.New(muxdb.NewMem(), trie.Root{})

	candidateList := generateCandidateList(5)

	// Call NewCandidates with the mock data
	candidates := NewCandidates(candidateList)

	endorsement, err := builtin.Params.Native(state).Get(thor.KeyProposerEndorsement)
	assert.NoError(t, err)

	checkBalance := func(master, endorser thor.Address) (bool, error) {
		bal, err := state.GetBalance(endorser)
		if err != nil {
			return false, err
		}
		return bal.Cmp(endorsement) >= 0, nil
	}

	proposers, err := candidates.Pick(state, checkBalance)

	assert.NoError(t, err)

	for i, proposer := range proposers {
		assert.Equal(t, proposer.Address, candidateList[i].NodeMaster, "NodeMaster must be contained in proposer")
		assert.Equal(t, proposer.Active, candidateList[i].Active, "NodeMaster must be active")
	}
}

func TestUpdate(t *testing.T) {
	candidateList := generateCandidateList(5)

	// Call NewCandidates with the mock data
	candidates := NewCandidates(candidateList)

	assert.True(t, candidates.Update(candidateList[0].NodeMaster, false), "Should return True")

	var newNodeMaster thor.Address
	rand.Read(newNodeMaster[:])

	assert.False(t, candidates.Update(newNodeMaster, false), "Should return false")
}
