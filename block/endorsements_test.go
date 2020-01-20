package block

import (
	"bytes"
	"fmt"
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

	endorsements.Add(eds[3])
	endorsements.Add(eds[1])
	endorsements.Add(eds[2])

	// endorsements.Swap(3, 0)
	fmt.Println(endorsements.StringVrfProofs())
	endorsements.Sort()
	fmt.Println(endorsements.StringVrfProofs())

	order := []int{2, 0, 1}
	proofs := endorsements.VrfProofs()
	for i, p := range proofs {
		if p != eds[order[i]].VrfProof() {
			t.Errorf("Incorrect Sort()")
			break
		}
	}

	var sigs []byte
	for _, ed := range eds {
		sigs = append(sigs, ed.Signature()...)
	}
	if bytes.Compare(sigs, endorsements.Signatures()) != 0 {
		t.Errorf("Incorrect Signatures()")
	}
}
