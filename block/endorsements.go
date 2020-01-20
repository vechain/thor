package block

import (
	"fmt"
	"sort"

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
func (eds *Endorsements) Add(ed *Endorsement) bool {
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

func (eds *Endorsements) Len() int { return len(eds.vals) }
func (eds *Endorsements) Swap(i, j int) {
	eds.vals[i], eds.vals[j] = eds.vals[j], eds.vals[i]
}

// Less returns true if the i'th VRF proof is less than the j'th VRF proof
func (eds *Endorsements) Less(i, j int) bool {
	iKey := eds.vals[i].VrfProof().Bytes()
	jKey := eds.vals[j].VrfProof().Bytes()

	for n, bi := range iKey {
		bj := jKey[n]
		if bi < bj {
			return true
		}
	}
	return false
}

// Sort sorts all the saved endorsements by their VRF proofs in an ascending order
func (eds *Endorsements) Sort() { sort.Sort(eds) }

// VrfProofs returns an array of VRF proofs
func (eds *Endorsements) VrfProofs() []*vrf.Proof {
	var proofs []*vrf.Proof
	for _, ed := range eds.vals {
		proofs = append(proofs, ed.VrfProof())
	}
	return proofs
}

// Signatures returns a combined byte array of signatures
func (eds *Endorsements) Signatures() []byte {
	var sigs []byte
	for _, ed := range eds.vals {
		sigs = append(sigs, ed.Signature()...)
	}
	return sigs
}

func (eds *Endorsements) StringVrfProofs() string {
	var s string
	for _, ed := range eds.vals {
		s = s + fmt.Sprintf("0x%x\n", ed.VrfProof().Bytes())
	}
	return s
}
