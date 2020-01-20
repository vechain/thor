package block

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

var sk, _ = crypto.GenerateKey()

func newEndorsement(bs *Summary, p []byte) *Endorsement {
	ed := NewEndorsement(bs, vrf.BytesToProof(p))
	sig, _ := crypto.Sign(ed.SigningHash().Bytes(), sk)
	return ed.WithSignature(sig)
}

func Test(t *testing.T) {
	bs := NewBlockSummary(thor.Bytes32{}, thor.Bytes32{}, 0)

	var endorsements Endorsements

	eds := []*Endorsement{
		newEndorsement(bs, []byte{0x2}),
		newEndorsement(bs, []byte{0x1, 0x1}),
		newEndorsement(bs, []byte{0x1}),
		newEndorsement(bs, []byte{0x1, 0x3}),
	}

	if !endorsements.Add(eds[0]) {
		t.Errorf("Add a new endorsement")
	}

	if endorsements.Add(eds[0]) {
		t.Errorf("Add a duplicated endorsement")
	}

	endorsements.Add(eds[1])
	endorsements.Add(eds[2])
	endorsements.Add(eds[3])

	proofs := endorsements.VrfProofs()
	for i, p := range proofs {
		_p := eds[i].VrfProof()
		if *p != *_p {
			t.Errorf("Incorrect VrfProofs()")
			break
		}
	}

	sigs := endorsements.Signatures()
	for i, ed := range eds {
		if bytes.Compare(sigs[i], ed.Signature()) != 0 {
			t.Errorf("Incorrect Signatures()")
			break
		}
	}
}
