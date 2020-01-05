package consensus

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"math"
	"testing"

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

func TestIsCommittee(t *testing.T) {
	_, sk := vrf.GenKeyPair()

	th := getCommitteeThreshold()

	var (
		msg       = make([]byte, 32)
		proof, pf *vrf.Proof
		err       error
	)

	// Get a positive sample
	for {
		rand.Read(msg)
		proof, err = sk.Prove(msg)
		if err != nil {
			t.Error(err)
		}
		hashedProof := thor.Blake2b(proof[:])
		v := binary.BigEndian.Uint32(hashedProof.Bytes())

		if v <= th {
			break
		}
	}

	pf, err = IsCommittee(sk, thor.BytesToBytes32(msg))
	// Test failed if 1) an error returned, 2) a nil proof returned, or
	// 3) the returned proof not equal to the precomputed one
	if err != nil || pf == nil || bytes.Compare(pf[:], proof[:]) != 0 {
		t.Errorf("Testing positive sample failed")
	}

	// Get a negative sample
	for {
		rand.Read(msg)
		proof, err = sk.Prove(msg)
		if err != nil {
			t.Error(err)
		}
		hashedProof := thor.Blake2b(proof[:])
		v := binary.BigEndian.Uint32(hashedProof.Bytes())

		if v > th {
			break
		}
	}

	pf, err = IsCommittee(sk, thor.BytesToBytes32(msg))
	// Test failed if 1) an error returned or 2) the returned proof not
	// equal to nil
	if err != nil || pf != nil {
		t.Errorf("Testing negative sample failed")
	}
}
