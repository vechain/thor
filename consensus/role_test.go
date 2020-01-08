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
	if cons.ValidateBlockSummary(bs, round) != nil {
		t.Errorf("Test1 failed")
	}

	// wrong round number
	if cons.ValidateBlockSummary(bs, round-1) != errRound {
		t.Errorf("Test2 failed")
	}

	// Wrong signature
	rand.Read(sig)
	bs = bs.WithSignature(sig)
	if cons.ValidateBlockSummary(bs, round) != errSig {
		t.Errorf("Test3 failed")
	}

	// wrong parentID
	bs = block.NewBlockSummary(best.Header().ParentID(), thor.BytesToBytes32([]byte(nil)), cons.Timestamp(round))
	sig, err = crypto.Sign(bs.SigningHash().Bytes(), privateKey)
	if err != nil {
		t.Fatal(err)
	}
	bs = bs.WithSignature(sig)
	if cons.ValidateBlockSummary(bs, round) != errParent {
		t.Errorf("Test4 failed")
	}
}

// func TestValidateEndorsement(t *testing.T) {
// 	pk, sk := vrf.GenKeyPair()
// 	msg := []byte("TestValidateEndorsement")
// 	proof, err := sk.Prove(msg)

// 	var ed block.Endorsement
// }

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
