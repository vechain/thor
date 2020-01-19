package block

import (
	"sort"

	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

// Endorsements manages endorsements
type Endorsements struct {
	keys map[thor.Bytes32]struct{}
	vals []*Endorsement
}

// Add store a new endorsement
func (eds Endorsements) Add(ed *Endorsement) bool {
	if eds.vals == nil {
		eds.vals = make([]*Endorsement, thor.CommitteeSize)
		eds.keys = make(map[thor.Bytes32]struct{})
	}

	// Check if it already exists
	if _, ok := eds.keys[ed.SigningHash()]; ok {
		return false
	}

	// Max the number of committee members required for creating a new block
	if len(eds.keys) >= int(thor.CommitteeSize) {
		return false
	}

	eds.keys[ed.SigningHash()] = struct{}{}
	eds.vals = append(eds.vals, ed)

	return true
}

func (eds Endorsements) Len() int { return len(eds.vals) }

func (eds Endorsements) Swap(i, j int) { eds.vals[i], eds.vals[j] = eds.vals[j], eds.vals[i] }

func (eds Endorsements) Less(i, j int) bool {
	iKey := eds.vals[i].SigningHash().Bytes()
	jKey := eds.vals[j].SigningHash().Bytes()

	for n, bi := range iKey {
		bj := jKey[n]
		if bi < bj {
			return true
		}
	}
	return false
}

// Sort ...
func (eds Endorsements) Sort() { sort.Sort(eds) }

// VrfProofs ...
func (eds Endorsements) VrfProofs() []*vrf.Proof {
	var proofs []*vrf.Proof
	for _, ed := range eds.vals {
		proofs = append(proofs, ed.VrfProof())
	}
	return proofs
}

// Signatures ...
func (eds Endorsements) Signatures() []byte {
	var sigs []byte
	for _, ed := range eds.vals {
		sigs = append(sigs, ed.Signature()...)
	}
	return sigs
}
