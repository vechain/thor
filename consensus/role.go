package consensus

import (
	"encoding/binary"
	"errors"
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

// func getCommitteeThreshold() uint32 {
// 	// threshold = (1 << 64 - 1) * committee_size / total_number_of_nodes * factor
// 	f := thor.CommitteeSize * thor.CommitteeThresholdFactor
// 	if f > thor.MaxBlockProposers {
// 		f = thor.MaxBlockProposers
// 	}
// 	return uint32(uint64(math.MaxUint32) * f / thor.MaxBlockProposers)
// }

// getCommitteeThreshold computes the max hash(vrf_proof) value for committee member.
//
// threshold is determined by the current number of qualified consensus nodes
// not the fixed max number of nodes allowed.
func (c *Consensus) getCommitteeThreshold() (uint32, error) {
	candidates, st, err := c.getAllCandidates(c.repo.BestBlock().Header())
	if err != nil {
		return 0, err
	}

	proposers, err := candidates.Pick(st)
	if err != nil {
		return 0, err
	}

	N := uint64(len(proposers))
	if N == 0 {
		return 0, errors.New("zero number of consensus nodes")
	}

	// if N < thor.CommitteeSize {
	// 	return 0, errors.New("number of consensus nodes less than the required committee size")
	// }

	f := thor.CommitteeSize * thor.CommitteeThresholdFactor
	if f > N {
		f = N
	}

	return uint32(uint64(math.MaxUint32) * f / N), nil
}

// IsCommittee checks the committeeship given a VRF private key and timestamp.
func (c *Consensus) IsCommittee(sk *vrf.PrivateKey, time uint64) (bool, *vrf.Proof, error) {
	round := c.RoundNumber(time)
	if round == 0 {
		return false, nil, newConsensusError(trNil, strErrZeroRound, nil, nil, "")
	}

	epoch := EpochNumber(round)
	beacon, err := c.beacon(epoch)
	if err != nil {
		return false, nil, err
	}

	seed := seed(beacon, round)
	th, err := c.getCommitteeThreshold()
	if err != nil {
		return false, nil, err
	}

	proof, err := sk.Prove(seed.Bytes())
	if isCommitteeByProof(proof, th) {
		return true, proof, nil
	}

	return false, nil, nil
}

// Seed computes the random seed for each round
//
// seed = H(epoch_seed || round_number)
func seed(beacon thor.Bytes32, round uint32) thor.Bytes32 {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, round)
	return thor.Blake2b(beacon.Bytes(), b)
}

// isCommitteeByProof checks committeeship based on vrf proof and threshold
func isCommitteeByProof(proof *vrf.Proof, th uint32) bool {
	h := thor.Blake2b(proof[:])
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
	launchTime := c.repo.GenesisBlock().Header().Timestamp()
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

// Timestamp computes timestamp given either round number (uint32) or local time (uint64)
func (c *Consensus) Timestamp(arg interface{}) uint64 {
	switch reflect.TypeOf(arg).String() {
	case "uint32":
		r := arg.(uint32)
		return c.repo.GenesisBlock().Header().Timestamp() + thor.BlockInterval*uint64(r)
	case "uint64":
		t := arg.(uint64)
		r := c.RoundNumber(t)
		return c.repo.GenesisBlock().Header().Timestamp() + thor.BlockInterval*uint64(r)
	default:
		panic("Need uint32 or uint64")
	}
}

// ValidateBlockSummary validates a block summary
//
// check:
// 1. timestamp
// 2. parentID
// 3. signature
// 4. leadership
func (c *Consensus) ValidateBlockSummary(bs *block.Summary, parentHeader *block.Header, now uint64) error {
	if bs == nil {
		return newConsensusError(trBlockSummary, "empty block summary", nil, nil, "")
	}

	// only validate block summary created in the current round
	if !c.isValidTimestamp(bs.Timestamp(), now) {
		return newConsensusError(trBlockSummary, strErrTimestamp,
			[]string{strDataTimestamp, strDataNowTime},
			[]interface{}{bs.Timestamp(), now}, "")
	}

	if bs.ParentID() != parentHeader.ID() {
		// return consensusError("Inconsistent parent block ID")
		return newConsensusError(trBlockSummary, strErrParentID,
			[]string{strDataLocal, strDataCurr},
			[]interface{}{parentHeader.ID().Abev(), bs.ParentID().Abev()}, "")
	}

	// Valdiate signature
	signer, err := bs.Signer()
	if err != nil {
		// return consensusError(fmt.Sprintf("Signer unavailable: %v", err))
		return newConsensusError(trBlockSummary, strErrSignature, nil, nil, err.Error())
	}

	// validate leader
	if _, err := c.validateLeader(signer, parentHeader, bs.Timestamp(), bs.TotalScore()); err != nil {
		return err.(consensusError).AddTraceInfo(trBlockSummary)
	}

	return nil
}

// ValidateTimestamp validates the input timestamp against the current time.
// The timestamp is often from the new block summary, tx set, endorsement or header
func (c *Consensus) isValidTimestamp(timestamp, now uint64) bool {
	round := c.RoundNumber(now)
	if round == 0 {
		return false
	}
	if timestamp != c.Timestamp(round) {
		return false
	}
	return true
}

// ValidateTxSet validates a tx set
//
// Check:
// 1. timestamp
// 2. signature
// 3. leadership
func (c *Consensus) ValidateTxSet(ts *block.TxSet, parentHeader *block.Header, now uint64) error {
	if ts == nil {
		return newConsensusError(trTxSet, "Empty tx set", nil, nil, "")
	}

	if !c.isValidTimestamp(ts.Timestamp(), now) {
		return newConsensusError(trTxSet, strErrTimestamp,
			[]string{strDataTimestamp, strDataNowTime},
			[]interface{}{ts.Timestamp(), now}, "")
	}

	// Valdiate signature
	signer, err := ts.Signer()
	if err != nil {
		// return consensusError(fmt.Sprintf("Signer unavailable: %v", err))
		return newConsensusError(trTxSet, strErrSignature, nil, nil, err.Error())
	}

	if _, err := c.validateLeader(signer, parentHeader, ts.Timestamp(), ts.TotalScore()); err != nil {
		return err.(consensusError).AddTraceInfo(trTxSet)
	}

	return nil
}

// ValidateEndorsement validates an endorsement
//
// check:
// 1. block summary
// 2. signature
// 3. registered consensus node
// 4. validity of vrf proof
// 5. committee
func (c *Consensus) ValidateEndorsement(ed *block.Endorsement, parentHeader *block.Header, now uint64) error {
	if ed == nil {
		return newConsensusError(trEndorsement, "Empty endorsement", nil, nil, "")
	}

	// validate the block summary
	if err := c.ValidateBlockSummary(ed.BlockSummary(), parentHeader, now); err != nil {
		return err.(consensusError).AddTraceInfo(trEndorsement)
	}

	// signature
	signer, err := ed.Signer()
	if err != nil {
		// return consensusError(fmt.Sprintf("Signer unvailable: %v", err))
		return newConsensusError(trEndorsement, strErrSignature, nil, nil, err.Error())
	}

	// check whether ed signer is a registered consensus node
	candidates, st, err := c.getAllCandidates(parentHeader)
	if err != nil {
		return newConsensusError(trEndorsement, "get all candidates", nil, nil, err.Error())
	}
	proposers, err := candidates.Pick(st)
	if err != nil {
		return newConsensusError(trEndorsement, "get valid consensus node list", nil, nil, err.Error())
	}

	var candidate *poa.Proposer
	for _, proposer := range proposers {
		if proposer.Address == signer {
			candidate = &proposer
			break
		}
	}
	if candidate == nil {
		// return consensusError("Signer not allowed to participate in consensus")
		return newConsensusError(trEndorsement, strErrNotCandidate,
			[]string{strDataAddr},
			[]interface{}{signer}, "")
	}

	if parentHeader.Number()+1 == c.forkConfig.VIP193 {
		vpk := thor.GetVrfPuiblicKey(candidate.Address)
		if vpk.IsZero() {
			return newConsensusError(trEndorsement, "failed to get the hard-coded vrf public key",
				[]string{strDataAddr}, []interface{}{candidate.Address}, "")
		}
		candidate.VrfPublicKey = vpk
	}

	// Compute random seed
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
		// return consensusError("Invalid vrf proof")
		return newConsensusError(trEndorsement, strErrProof, nil, nil, "")
	}

	// validate committeeship
	th, err := c.getCommitteeThreshold()
	if err != nil {
		return err
	}
	if !isCommitteeByProof(ed.VrfProof(), th) {
		// return consensusError("Not a committee member")
		return newConsensusError(trEndorsement, strErrNotCommittee, nil, nil, "")
	}

	return nil
}

// validateLeader validates leadership
//
// check:
// 1. signature
// 2. leadership
// 3. total score
func (c *Consensus) validateLeader(signer thor.Address, parentHeader *block.Header, timestamp, totalScore uint64) (*poa.Candidates, error) {
	candidates, st, err := c.getAllCandidates(parentHeader)
	if err != nil {
		return nil, err
	}

	proposers, err := candidates.Pick(st)
	if err != nil {
		return nil, err
	}

	sched, err := poa.NewScheduler(signer, proposers, parentHeader.Number(), parentHeader.Timestamp())
	if err != nil {
		// return nil, consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
		return nil, newConsensusError(trLeader, strErrSigner,
			[]string{strDataAddr},
			[]interface{}{signer}, err.Error())
	}

	if !sched.IsTheTime(timestamp) {
		// return nil, consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", timestamp, signer))
		return nil, newConsensusError(trLeader, strErrTimestampUnsched,
			[]string{strDataTimestamp, strDataAddr},
			[]interface{}{timestamp, signer}, "")
	}

	_, score := sched.Updates(timestamp)
	if parentHeader.TotalScore()+score != totalScore {
		// return nil, consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parentHeader.TotalScore()+score, totalScore))
		return nil, newConsensusError(trLeader, strErrTotalScore,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{parentHeader.TotalScore() + score, totalScore}, "")
	}

	// for _, u := range updates {
	// 	authority.Update2(u.Address, u.Active)
	// 	if !candidates.Update(u.Address, u.Active) {
	// 		// should never happen
	// 		panic("something wrong with candidates list")
	// 	}
	// }

	return candidates, nil
}

func (c *Consensus) getAllCandidates(header *block.Header) (*poa.Candidates, *state.State, error) {
	st := c.stater.NewState(header.StateRoot())

	aut := builtin.Authority.Native(st)
	var candidates *poa.Candidates
	// if entry, ok := c.candidatesCache.Get(header.ID()); ok {
	// 	candidates = entry.(*poa.Candidates).Copy()
	// } else {
	var (
		list []*authority.Candidate
		err  error
	)

	vip193 := c.forkConfig.VIP193
	if vip193 == 0 {
		vip193 = 1
	}
	if header.Number() < vip193 {
		list, err = aut.AllCandidates()
	} else {
		list, err = aut.AllCandidates2()
	}
	if err != nil {
		return nil, nil, err
	}
	candidates = poa.NewCandidates(list)
	// }

	return candidates, st, nil
}
