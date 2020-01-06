package consensus

import (
	"encoding/binary"
	"math"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

// Role consensus role type
type Role uint8

// Define different roles
const (
	Leader Role = iota
	Committee
	Normal
)

func getCommitteeThreshold() uint32 {
	// threshold = (1 << 64 - 1) * committee_size / total_number_of_nodes * factor
	return uint32(uint64(math.MaxUint32) / thor.MaxBlockProposers * thor.CommitteeSize * thor.CommitteeThresholdFactor)
}

// IsCommittee checks committeeship. proof == nil -> false, otherwise true.
func IsCommittee(sk *vrf.PrivateKey, seed thor.Bytes32) (*vrf.Proof, error) {
	// Compute VRF proof
	proof, err := sk.Prove(seed.Bytes())
	if err != nil {
		return nil, err
	}

	// Compute the hash of the proof
	h := thor.Blake2b(proof[:])
	// Get the threshold
	th := getCommitteeThreshold()
	// Is a committee member if the hash is no larger than the threshold
	if binary.BigEndian.Uint32(h.Bytes()) <= th {
		return proof, nil
	}
	return nil, nil
}

// CompRoundSeed computes the random seed for each round
func CompRoundSeed(beacon thor.Bytes32, roundNum uint32) thor.Bytes32 {
	// round_seed = H(epoch_seed || round_number)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, roundNum)
	return thor.Blake2b(beacon.Bytes(), b)
}

// IsLeader checks whether the input address is the leader
func (c *Consensus) IsLeader(thor.Address) bool {
	return false
}

// ValidateBlockSummary validates a given block summary
func (c *Consensus) ValidateBlockSummary(bs *block.Summary) bool {
	return false
}
