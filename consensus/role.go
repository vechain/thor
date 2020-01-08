package consensus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

func getCommitteeThreshold() uint32 {
	// threshold = (1 << 64 - 1) * committee_size / total_number_of_nodes * factor
	return uint32(uint64(math.MaxUint32) / thor.MaxBlockProposers * thor.CommitteeSize * thor.CommitteeThresholdFactor)
}

// IsCommittee checks the committeeship given a VRF private key and round number.
func (c *Consensus) IsCommittee(sk *vrf.PrivateKey, round uint32) (bool, *vrf.Proof, error) {
	epoch, err1 := EpochNumber(round)
	if err1 != nil {
		return false, nil, err1
	}

	beacon, err2 := c.beacon(epoch)
	if err2 != nil {
		return false, nil, err2
	}

	seed := seed(beacon, round)
	return isCommitteeByPrivateKey(sk, seed)
}

// Seed computes the random seed for each round
func seed(beacon thor.Bytes32, round uint32) thor.Bytes32 {
	// round_seed = H(epoch_seed || round_number)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, round)
	return thor.Blake2b(beacon.Bytes(), b)
}

func isCommitteeByPrivateKey(sk *vrf.PrivateKey, seed thor.Bytes32) (bool, *vrf.Proof, error) {
	// Compute VRF proof
	proof, err := sk.Prove(seed.Bytes())
	if err != nil {
		return false, nil, err
	}

	// // Compute the hash of the proof
	// h := thor.Blake2b(proof[:])
	// // Get the threshold
	// th := getCommitteeThreshold()
	// // Is a committee member if the hash is no larger than the threshold
	// if binary.BigEndian.Uint32(h.Bytes()) <= th {
	// 	return proof, nil
	// }

	if isCommitteeByProof(proof) {
		return true, proof, nil
	}

	return false, nil, nil
}

func isCommitteeByProof(proof *vrf.Proof) bool {
	// Compute the hash of the proof
	h := thor.Blake2b(proof[:])
	// Get the threshold
	th := getCommitteeThreshold()
	// Is a committee member if the hash is no larger than the threshold
	if binary.BigEndian.Uint32(h.Bytes()) <= th {
		return true
	}
	return false
}

// IsLeader checks whether the input address is the leader
func (c *Consensus) IsLeader(thor.Address) bool {
	return false
}

// ValidateBlockSummary validates a given block summary
func (c *Consensus) ValidateBlockSummary(bs *block.Summary, round uint32) error {
	// Check timestamp
	t := bs.Timestamp()
	if t != uint64(round)*thor.BlockInterval+c.chain.GenesisBlock().Header().Timestamp() {
		return errRound
	}

	parent := c.chain.BestBlock().Header().ID()
	if bytes.Compare(parent.Bytes(), bs.ParentID().Bytes()) != 0 {
		return errParent
	}

	// Check signature
	if _, err := bs.Signer(); err != nil {
		return errSig
	}

	return nil
}

// ValidateEndorsement validates a given endorsement
func (c *Consensus) ValidateEndorsement(ed *block.Endorsement, round uint32) error {
	if err := c.ValidateBlockSummary(ed.BlockSummary(), round); err != nil {
		return err
	}

	epoch, err := EpochNumber(round)
	if err != nil {
		return err
	}

	beacon, err := c.beacon(epoch)
	if err != nil {
		return err
	}
	seed := seed(beacon, round)
	if ok, _ := ed.VrfPublicKey().Verify(ed.VrfProof(), seed.Bytes()); !ok {
		return errors.New("Verification failed")
	}

	if isCommitteeByProof(ed.VrfProof()) {
		return errors.New("Invalid committeeship")
	}
	return nil
}

// RoundNumber computes the round number from timestamp
func (c *Consensus) RoundNumber(timestamp uint64) (uint32, error) {
	launchTime := c.chain.GenesisBlock().Header().Timestamp()
	if launchTime+thor.BlockInterval > timestamp {
		return 0, errTimestamp
	}
	return uint32((timestamp - launchTime) / thor.BlockInterval), nil
}

// EpochNumber computes the epoch number from timestamp
func (c *Consensus) EpochNumber(timestamp uint64) (uint32, error) {
	round, err := c.RoundNumber(timestamp)
	if err != nil {
		return 0, err
	}
	return uint32(uint64(round-1)/thor.EpochInterval + 1), nil
}

// EpochNumber computes the epoch number from time
func EpochNumber(round uint32) (uint32, error) {
	if round == 0 {
		return 0, errZeroRound
	}
	return uint32(uint64(round-1)/thor.EpochInterval + 1), nil
}

// Timestamp computes the timestamp for the block generated in that round
func (c *Consensus) Timestamp(round uint32) uint64 {
	return c.chain.GenesisBlock().Header().Timestamp() + thor.BlockInterval*uint64(round)
}
