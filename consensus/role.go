package consensus

import (
	"encoding/binary"
	"math"

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

func getCommitteeThreshold() uint64 {
	// threshold = (1 << 64 - 1) * committee_size / total_number_of_nodes * factor
	return uint64(math.MaxUint32) / thor.MaxBlockProposers * thor.CommitteeSize * thor.CommitteeThresholdFactor
}

// GetEpochSeed computes the random seed for each epoch
func (c *Consensus) GetEpochSeed(currEpochNum uint32) (thor.Bytes32, error) {
	// Get the potential number of the last block in the last epoch
	blockNum := (currEpochNum - 1) * uint32(thor.EpochInterval)

	// For the first epoch, use the id of the genesis block as the seed
	if blockNum == 0 {
		return c.chain.GenesisBlock().Header().ID(), nil
	}

	seeker := c.chain.NewSeeker(c.chain.BestBlock().Header().ID())

	// Try to get the id of the last block of the last epoch
	id := seeker.GetID(blockNum)
	err := seeker.Err()
	for i := uint32(1); err != nil; i++ {
		// If not found, try to get the id of the previous block
		id = seeker.GetID(blockNum - i)
	}

	// Return block ID for the moment
	return id, nil
}

// GetRoundSeed computes the random seed for each round
func GetRoundSeed(epochSeed thor.Bytes32, roundNum uint32) thor.Bytes32 {
	// round_seed = H(epoch_seed || round_number)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, roundNum)
	return thor.Blake2b(epochSeed.Bytes(), b)
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
	if binary.BigEndian.Uint64(h.Bytes()) <= th {
		return proof, nil
	}
	return nil, nil
}
