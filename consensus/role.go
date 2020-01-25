package consensus

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

func getCommitteeThreshold() uint32 {
	// threshold = (1 << 64 - 1) * committee_size / total_number_of_nodes * factor
	return uint32(uint64(math.MaxUint32) / thor.MaxBlockProposers * thor.CommitteeSize * thor.CommitteeThresholdFactor)
}

// IsCommittee checks the committeeship given a VRF private key and round number.
func (c *Consensus) IsCommittee(sk *vrf.PrivateKey, round uint32) (bool, *vrf.Proof, error) {
	if round == 0 {
		return false, nil, errZeroRound
	}

	epoch := EpochNumber(round)
	beacon, err := c.beacon(epoch)
	if err != nil {
		return false, nil, err
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

// IsCommitteeByProof ...
func IsCommitteeByProof(proof *vrf.Proof) bool {
	return isCommitteeByProof(proof)
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

// RoundNumber computes the round number from timestamp
func (c *Consensus) RoundNumber(timestamp uint64) (uint32, error) {
	launchTime := c.chain.GenesisBlock().Header().Timestamp()
	if launchTime > timestamp {
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

	if round == 0 {
		return 0, nil
	}

	return uint32(uint64(round-1)/thor.EpochInterval + 1), nil
}

// EpochNumber computes the epoch number from time
func EpochNumber(round uint32) uint32 {
	if round == 0 {
		return 0
	}
	return uint32(uint64(round-1)/thor.EpochInterval + 1)
}

// RoundNumber ...
func RoundNumber(now, launch uint64) (uint32, error) {
	if launch > now {
		return 0, errTimestamp
	}
	return uint32((now - launch) / thor.BlockInterval), nil
}

// Timestamp computes the timestamp for the block generated in that round
func (c *Consensus) Timestamp(round uint32) uint64 {
	return c.chain.GenesisBlock().Header().Timestamp() + thor.BlockInterval*uint64(round)
}

// ValidateBlockSummary validates a given block summary
func (c *Consensus) ValidateBlockSummary(bs *block.Summary, parent *block.Header, now uint64) error {
	if bs.ParentID() != parent.ID() {
		return consensusError("Inconsistent parent block ID")
	}

	st, err := c.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return err
	}

	_, err = c.validateLeader(bs, parent, st)
	if err != nil {
		return err
	}

	return nil
}

// ValidateEndorsement validates a given endorsement
func (c *Consensus) ValidateEndorsement(ed *block.Endorsement, vrfPublicKey *vrf.PublicKey, parent *block.Header, now uint64) error {
	// validate the block summary
	if err := c.ValidateBlockSummary(ed.BlockSummary(), parent, now); err != nil {
		return err
	}

	// Compute the VRF seed
	round, _ := c.RoundNumber(ed.BlockSummary().Timestamp())
	epoch := EpochNumber(round)
	beacon, err := c.beacon(epoch)
	if err != nil {
		return err
	}
	seed := seed(beacon, round)

	// validate proof
	if ok, _ := vrfPublicKey.Verify(ed.VrfProof(), seed.Bytes()); !ok {
		return consensusError("Invalid vrf proof")
	}

	// validate committeeship
	if !isCommitteeByProof(ed.VrfProof()) {
		return consensusError("Not a committee member")
	}

	return nil
}

// validate:
// 1. signature
// 2. leadership
// 3. total score
func (c *Consensus) validateLeader(bs *block.Summary, parent *block.Header, st *state.State) (*poa.Candidates, error) {
	signer, err := bs.Signer()
	if err != nil {
		return nil, consensusError(fmt.Sprintf("block signer unavailable: %v", err))
	}

	authority := builtin.Authority.Native(st)
	var candidates *poa.Candidates
	if entry, ok := c.candidatesCache.Get(parent.ID()); ok {
		candidates = entry.(*poa.Candidates).Copy()
	} else {
		candidates = poa.NewCandidates(authority.AllCandidates())
	}

	proposers := candidates.Pick(st)

	sched, err := poa.NewScheduler(signer, proposers, parent.Number(), parent.Timestamp())
	if err != nil {
		return nil, consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
	}

	if !sched.IsTheTime(bs.Timestamp()) {
		return nil, consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", bs.Timestamp(), signer))
	}

	updates, score := sched.Updates(bs.Timestamp())
	if parent.TotalScore()+score != bs.TotalScore() {
		return nil, consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parent.TotalScore()+score, bs.TotalScore()))
	}

	for _, u := range updates {
		authority.Update(u.Address, u.Active)
		if !candidates.Update(u.Address, u.Active) {
			// should never happen
			panic("something wrong with candidates list")
		}
	}

	return candidates, nil
}
