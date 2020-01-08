package consensus

import (
	"bytes"
	"crypto/rand"
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

func TestThreshold(t *testing.T) {
	th := getCommitteeThreshold()
	// ratio = threhsold / (1 << 32 - 1) <= amp_factor * #committee / #node
	ratio := float64(th) / float64(math.MaxUint32)
	if ratio > float64(thor.CommitteeSize)/float64(thor.MaxBlockProposers)*float64(thor.CommitteeThresholdFactor) {
		t.Errorf("Invalid threshold")
	}
}

func TestIsCommitteeByPrivateKey(t *testing.T) {
	_, sk := vrf.GenKeyPair()

	// th := getCommitteeThreshold()

	var (
		msg       = make([]byte, 32)
		proof, pf *vrf.Proof
		err       error
		ok        bool
	)

	// Get a positive sample
	for {
		rand.Read(msg)
		proof, err = sk.Prove(msg)
		if err != nil {
			t.Error(err)
		}

		if isCommitteeByProof(proof) {
			break
		}
	}

	ok, pf, err = isCommitteeByPrivateKey(sk, thor.BytesToBytes32(msg))
	if err != nil || !ok || pf == nil || bytes.Compare(pf[:], proof[:]) != 0 {
		t.Errorf("Testing positive sample failed")
	}

	// Get a negative sample
	for {
		rand.Read(msg)
		proof, err = sk.Prove(msg)
		if err != nil {
			t.Error(err)
		}

		if !isCommitteeByProof(proof) {
			break
		}
	}

	ok, pf, err = isCommitteeByPrivateKey(sk, thor.BytesToBytes32(msg))
	if err != nil || ok || pf != nil {
		t.Errorf("Testing negative sample failed")
	}
}

func TestRoundNumber(t *testing.T) {
	cons := initConsensus()
	launchTime := cons.chain.GenesisBlock().Header().Timestamp()

	var (
		round uint32
		err   error
	)

	round, err = cons.RoundNumber(launchTime + thor.BlockInterval - 1)
	if err == nil {
		t.Errorf("Test1 failed")
	}

	round, err = cons.RoundNumber(launchTime + thor.BlockInterval*10)
	if round != 10 || err != nil {
		t.Errorf("Test2 failed")
	}

	round, err = cons.RoundNumber(launchTime + thor.BlockInterval*10 + 1)
	if round != 10 || err != nil {
		t.Errorf("Test2 failed")
	}
}

func TestEpochNumber(t *testing.T) {
	cons := initConsensus()
	launchTime := cons.chain.GenesisBlock().Header().Timestamp()

	var (
		epoch uint32
		err   error
	)

	// timestamp eailer than the first block
	epoch, err = cons.EpochNumber(launchTime + thor.BlockInterval - 1)
	if err == nil {
		t.Errorf("Test1 failed")
	}

	// round epoch_interval-1
	epoch, err = cons.EpochNumber(launchTime + thor.BlockInterval*(thor.EpochInterval-1))
	if epoch != 1 || err != nil {
		t.Errorf("Test2 failed")
	}

	// round epoch_interval
	epoch, err = cons.EpochNumber(launchTime + thor.BlockInterval*thor.EpochInterval)
	if epoch != 1 || err != nil {
		t.Errorf("Test3 failed")
	}

	// round epoch_inverval+1
	epoch, err = cons.EpochNumber(launchTime + thor.BlockInterval*(thor.EpochInterval+1))
	if epoch != 2 || err != nil {
		t.Errorf("Test4 failed")
	}
}

func TestValidateBlockSummary(t *testing.T) {
	privateKey, _ := crypto.GenerateKey()

	cons := initConsensus()
	nRound := uint32(10)
	addEmptyBlocks(cons.chain, privateKey, nRound, make(map[uint32]interface{}))

	best := cons.chain.BestBlock()
	round := nRound + 1

	var (
		bs  *block.Summary
		sig []byte
		err error
	)

	// clean case
	bs = block.NewBlockSummary(best.Header().ID(), thor.BytesToBytes32([]byte(nil)), cons.Timestamp(round))
	sig, err = crypto.Sign(bs.SigningHash().Bytes(), privateKey)
	if err != nil {
		t.Fatal(err)
	}
	bs = bs.WithSignature(sig)
	if cons.ValidateBlockSummary(bs) != nil {
		t.Errorf("clean case failed")
	}

	// // Wrong signature
	// rand.Read(sig)
	// bs = bs.WithSignature(sig)
	// if cons.ValidateBlockSummary(bs) != errSig {
	// 	t.Errorf("Test3 failed")
	// }

	// wrong parentID
	bs = block.NewBlockSummary(best.Header().ParentID(), thor.BytesToBytes32([]byte(nil)), cons.Timestamp(round))
	sig, err = crypto.Sign(bs.SigningHash().Bytes(), privateKey)
	if err != nil {
		t.Fatal(err)
	}
	bs = bs.WithSignature(sig)
	if cons.ValidateBlockSummary(bs) != errParent {
		t.Errorf("errParant failed")
	}

	// wrong timestamp
	bs = block.NewBlockSummary(best.Header().ID(), thor.BytesToBytes32([]byte(nil)), cons.Timestamp(round)-1)
	sig, err = crypto.Sign(bs.SigningHash().Bytes(), privateKey)
	if err != nil {
		t.Fatal(err)
	}
	bs = bs.WithSignature(sig)
	if cons.ValidateBlockSummary(bs) != errTimestamp {
		t.Errorf("errTimestamp failed")
	}
}

func getValidCommittee(seed thor.Bytes32) (*vrf.Proof, *vrf.PublicKey) {
	maxIter := 1000
	for i := 0; i < maxIter; i++ {
		pk, sk := vrf.GenKeyPair()
		proof, _ := sk.Prove(seed.Bytes())
		if isCommitteeByProof(proof) {
			return proof, pk
		}
	}
	return nil, nil
}

func getInvalidCommittee(seed thor.Bytes32) (*vrf.Proof, *vrf.PublicKey) {
	maxIter := 1000
	for i := 0; i < maxIter; i++ {
		pk, sk := vrf.GenKeyPair()
		proof, _ := sk.Prove(seed.Bytes())
		if !isCommitteeByProof(proof) {
			return proof, pk
		}
	}
	return nil, nil
}

func TestValidateEndorsement(t *testing.T) {
	ethsk, _ := crypto.GenerateKey()

	cons := initConsensus()
	gen := cons.chain.GenesisBlock().Header()

	// Create a valid block summary at round 1
	bs := block.NewBlockSummary(gen.ID(), thor.BytesToBytes32([]byte(nil)), gen.Timestamp()+thor.BlockInterval)
	sig, err := crypto.Sign(bs.SigningHash().Bytes(), ethsk)
	if err != nil {
		t.Fatal(err)
	}
	bs = bs.WithSignature(sig)

	// Get the committee keys and proof
	beacon := getBeaconFromHeader(cons.chain.GenesisBlock().Header())
	seed := seed(beacon, 1)
	proof, pk := getValidCommittee(seed)
	if proof == nil {
		t.Errorf("Failed to find a valid committee")
	}

	// Clean case
	ed1 := block.NewEndorsement(bs, pk, proof)
	sig, err = crypto.Sign(ed1.SigningHash().Bytes(), ethsk)
	if err != nil {
		t.Fatal(err)
	}
	ed1 = ed1.WithSignature(sig)
	if err := cons.ValidateEndorsement(ed1); err != nil {
		t.Errorf("clean case")
	}

	// // wrong signature
	// randSig := make([]byte, 65)
	// rand.Read(randSig)
	// ed1 = ed1.WithSignature(randSig)
	// if err := cons.ValidateEndorsement(ed1); err != errSig {
	// 	t.Errorf("Test3 failed")
	// }

	// wrong proof
	var randProof vrf.Proof
	rand.Read(randProof[:])
	ed2 := block.NewEndorsement(bs, pk, &randProof)
	sig, err = crypto.Sign(ed1.SigningHash().Bytes(), ethsk)
	if err != nil {
		t.Fatal(err)
	}
	ed2 = ed2.WithSignature(sig)
	if err := cons.ValidateEndorsement(ed2); err != errVrfProof {
		t.Errorf("errVrfProof")
	}

	// not committee
	proof, pk = getInvalidCommittee(seed)
	if proof == nil {
		t.Errorf("Failed to find a false committee")
	}
	ed3 := block.NewEndorsement(bs, pk, proof)
	sig, err = crypto.Sign(seed.Bytes(), ethsk)
	if err != nil {
		t.Fatal(err)
	}
	ed3 = ed3.WithSignature(sig)
	if err := cons.ValidateEndorsement(ed3); err != errNotCommittee {
		t.Errorf("errNotCommittee")
	}
}

func BenchmarkTestEthSig(b *testing.B) {
	sk, _ := crypto.GenerateKey()

	msg := make([]byte, 32)

	for i := 0; i < b.N; i++ {
		rand.Read(msg)
		crypto.Sign(msg, sk)
	}
}

func BenchmarkBeacon(b *testing.B) {
	cons := initConsensus()

	for i := 0; i < b.N; i++ {
		cons.beacon(uint32(i + 1))
	}
}
