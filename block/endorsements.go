package block

import (
	"fmt"

	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

// Endorsements manages endorsements
type Endorsements struct {
	keys map[thor.Bytes32]struct{}
	vals []*Endorsement
}

// Add adds a new endorsement. It returns false if the endorsement already exists.
// Endorsements are distinguished by their signing hashes.
func (eds *Endorsements) AddEndorsement(ed *Endorsement) bool {
	if eds.vals == nil {
		// eds.vals = make([]*Endorsement, 1)
		eds.keys = make(map[thor.Bytes32]struct{})
	}

	// Check if it already exists
	if _, ok := eds.keys[ed.SigningHash()]; ok {
		return false
	}

	eds.keys[ed.SigningHash()] = struct{}{}
	eds.vals = append(eds.vals, ed)

	return true
}

// VrfProofs returns an array of VRF proofs
func (eds *Endorsements) VrfProofs() []*vrf.Proof {
	var proofs []*vrf.Proof
	for _, ed := range eds.vals {
		proofs = append(proofs, ed.VrfProof().Copy())
	}
	return proofs
}

// Signatures returns a combined byte array of signatures
func (eds *Endorsements) Signatures() [][]byte {
	var sigs [][]byte
	for _, ed := range eds.vals {
		sigs = append(sigs, append([]byte(nil), ed.Signature()...))
	}
	return sigs
}

// StringVrfProofs ...
func (eds *Endorsements) StringVrfProofs() string {
	var s string
	for _, ed := range eds.vals {
		// b := ed.VrfProof().Bytes()
		s = s + fmt.Sprintf("%x\n", *ed.VrfProof())
	}
	return s
}

// Len ...
func (eds *Endorsements) Len() int {
	return len(eds.vals)
}

func (eds *Endorsements) List() []*Endorsement {
	return eds.vals
}
