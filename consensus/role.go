package consensus

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/builtin/authority"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

func getCommitteeThreshold() uint32 {
	// threshold = (1 << 64 - 1) * committee_size / total_number_of_nodes * factor
	f := thor.CommitteeSize * thor.CommitteeThresholdFactor
	if f > thor.MaxBlockProposers {
		f = thor.MaxBlockProposers
	}

	return uint32(uint64(math.MaxUint32) * f / thor.MaxBlockProposers)
}

// IsCommittee checks the committeeship given a VRF private key and round number.
func (c *Consensus) IsCommittee(sk *vrf.PrivateKey, time uint64) (bool, *vrf.Proof, error) {
	round := c.RoundNumber(time)
	if round == 0 {
		return false, nil, consensusError("Zero round number")
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
func (c *Consensus) RoundNumber(t uint64) uint32 {
	launchTime := c.chain.GenesisBlock().Header().Timestamp()
	if launchTime > t {
		return 0
	}
	return uint32((t - launchTime) / thor.BlockInterval)
}

// EpochNumber computes the epoch number from timestamp
func (c *Consensus) EpochNumber(timestamp uint64) uint32 {
	round := c.RoundNumber(timestamp)

	if round == 0 {
		return 0
	}

	return uint32(uint64(round-1)/thor.EpochInterval + 1)
}

// EpochNumber computes the epoch number from time
func EpochNumber(round uint32) uint32 {
	if round == 0 {
		return 0
	}
	return uint32(uint64(round-1)/thor.EpochInterval + 1)
}

// RoundNumber ...
func RoundNumber(now, launch uint64) uint32 {
	if launch > now {
		return 0
	}
	return uint32((now - launch) / thor.BlockInterval)
}

// Timestamp computes the round timestamp.
// [arg] can be either round number (uint32) or time (uint64)
func (c *Consensus) Timestamp(arg interface{}) uint64 {
	switch reflect.TypeOf(arg).String() {
	case "uint32":
		r := arg.(uint32)
		return c.chain.GenesisBlock().Header().Timestamp() + thor.BlockInterval*uint64(r)
	case "uint64":
		t := arg.(uint64)
		r := c.RoundNumber(t)
		return c.chain.GenesisBlock().Header().Timestamp() + thor.BlockInterval*uint64(r)
	default:
		panic("Need uint32 or uint64")
	}
}

// // NextTimestamp computes the next-round timestamp
// func (c *Consensus) NextTimestamp(t uint64) uint64 {
// 	r := c.RoundNumber(t + thor.BlockInterval)
// 	return c.chain.GenesisBlock().Header().Timestamp() + thor.BlockInterval*uint64(r)
// }

// // TimestampFromCurrTime ...
// func (c *Consensus) Timestamp(now uint64) uint64 {
// 	return c.Timestamp(c.RoundNumber(now))
// }

// ValidateBlockSummary validates a block summary
func (c *Consensus) ValidateBlockSummary(bs *block.Summary, parentHeader *block.Header, now uint64) error {
	if bs == nil {
		return consensusError("Empty block summary")
	}

	// only validate block summary created in the current round
	if err := c.validateTimestamp(bs.Timestamp(), now); err != nil {
		return err
	}

	if bs.ParentID() != parentHeader.ID() {
		return consensusError("Inconsistent parent block ID")
	}

	// Valdiate signature
	signer, err := bs.Signer()
	if err != nil {
		return consensusError(fmt.Sprintf("Signer unavailable: %v", err))
	}

	// validate leader
	if _, err := c.validateLeader(signer, parentHeader, bs.Timestamp(), bs.TotalScore()); err != nil {
		return err
	}

	return nil
}

// ValidateTimestamp validates the input timestamp against the current time.
// The timestamp is often from the new block summary, tx set, endorsement or header
func (c *Consensus) validateTimestamp(timestamp, now uint64) error {
	round := c.RoundNumber(now)
	if round == 0 {
		return consensusError("Zero round number")
	}
	if timestamp != c.Timestamp(round) {
		return consensusError(fmt.Sprintf("Invalid timestamp, timestamp=%v, now=%v", timestamp, now))
	}
	return nil
}

// ValidateTxSet validates a tx set
func (c *Consensus) ValidateTxSet(ts *block.TxSet, parentHeader *block.Header, now uint64) error {
	if ts == nil {
		return consensusError("Empty tx set")
	}

	if err := c.validateTimestamp(ts.Timestamp(), now); err != nil {
		return err
	}

	// Valdiate signature
	signer, err := ts.Signer()
	if err != nil {
		return consensusError(fmt.Sprintf("Signer unavailable: %v", err))
	}

	if _, err := c.validateLeader(signer, parentHeader, ts.Timestamp(), ts.TotalScore()); err != nil {
		return err
	}

	return nil
}

// ValidateEndorsement validates an endorsement
func (c *Consensus) ValidateEndorsement(ed *block.Endorsement, parentHeader *block.Header, now uint64) error {
	if ed == nil {
		return consensusError("Empty endorsement")
	}

	// validate the block summary
	if err := c.ValidateBlockSummary(ed.BlockSummary(), parentHeader, now); err != nil {
		return err
	}

	candidates, _, st, err := c.getAllCandidates(parentHeader)
	if err != nil {
		return err
	}

	signer, err := ed.Signer()
	if err != nil {
		return consensusError(fmt.Sprintf("Signer unvailable: %v", err))
	}

	fmt.Println(candidates.String())
	fmt.Printf("%x\n", signer)

	candidate := candidates.Candidate(st, signer)
	if candidate == nil {
		return consensusError("Signer not allowed to participate in consensus")
	}

	// Compute the VRF seed
	round := c.RoundNumber(ed.BlockSummary().Timestamp())
	epoch := EpochNumber(round)
	beacon, err := c.beacon(epoch)
	if err != nil {
		return err
	}
	seed := seed(beacon, round)

	// validate proof
	vrfPubkey := vrf.Bytes32ToPublicKey(candidate.VrfPublicKey)
	if ok, _ := vrfPubkey.Verify(ed.VrfProof(), seed.Bytes()); !ok {
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
func (c *Consensus) validateLeader(signer thor.Address, parentHeader *block.Header, timestamp, totalScore uint64) (*poa.Candidates, error) {
	candidates, authority, st, err := c.getAllCandidates(parentHeader)
	if err != nil {
		return nil, err
	}
	proposers := candidates.Pick(st)

	sched, err := poa.NewScheduler(signer, proposers, parentHeader.Number(), parentHeader.Timestamp())
	if err != nil {
		return nil, consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
	}

	if !sched.IsTheTime(timestamp) {
		return nil, consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", timestamp, signer))
	}

	updates, score := sched.Updates(timestamp)
	if parentHeader.TotalScore()+score != totalScore {
		return nil, consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parentHeader.TotalScore()+score, totalScore))
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

func (c *Consensus) getAllCandidates(parentHeader *block.Header) (*poa.Candidates, *authority.Authority, *state.State, error) {
	st, err := c.stateCreator.NewState(parentHeader.StateRoot())
	if err != nil {
		return nil, nil, nil, err
	}

	authority := builtin.Authority.Native(st)
	var candidates *poa.Candidates
	if entry, ok := c.candidatesCache.Get(parentHeader.ID()); ok {
		candidates = entry.(*poa.Candidates).Copy()
	} else {
		candidates = poa.NewCandidates(authority.AllCandidates())
		c.candidatesCache.Add(parentHeader.ID(), candidates)
	}

	return candidates, authority, st, nil
}
