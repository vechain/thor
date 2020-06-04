// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

var (
	emptyRoot = trie.DeriveRoot(&derivableApprovals{})
)

// Approval is the approval of a block.
type Approval struct {
	body struct {
		PublicKey []byte // Compressed public key
		Proof     []byte
	}
	cache struct {
		hash   atomic.Value
		signer atomic.Value
		beta   atomic.Value
	}
}

// NewApproval creates a new approval.
func NewApproval(pub, proof []byte) *Approval {
	var a Approval
	a.body.PublicKey = pub
	a.body.Proof = proof

	return &a
}

// Hash is the hash of RLP encoded approval.
func (a *Approval) Hash() (hash thor.Bytes32) {
	if cached := a.cache.hash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { a.cache.hash.Store(hash) }()

	hw := thor.NewBlake2b()
	rlp.Encode(hw, a)
	hw.Sum(hash[:0])
	return
}

// Validate validates approval's proof and returns the VRF output.
func (a *Approval) Validate(alpha []byte) (beta []byte, err error) {
	if cached := a.cache.beta.Load(); cached != nil {
		return cached.([]byte), nil
	}
	defer func() { a.cache.beta.Store(beta) }()

	vrf := ecvrf.NewSecp256k1Sha256Tai()
	pub, err := crypto.DecompressPubkey(a.body.PublicKey)
	if err != nil {
		return []byte{}, err
	}
	beta, err = vrf.Verify(pub, alpha, a.body.Proof)
	return
}

// Signer computes the address from the public key.
func (a *Approval) Signer() (signer thor.Address, err error) {
	if cached := a.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() { a.cache.signer.Store(signer) }()

	pub, err := crypto.DecompressPubkey(a.body.PublicKey)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// EncodeRLP implements rlp.Encoder.
func (a *Approval) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &a.body)
}

// DecodeRLP implements rlp.Decoder.
func (a *Approval) DecodeRLP(s *rlp.Stream) error {
	var body struct {
		PublicKey []byte
		Proof     []byte
	}

	if err := s.Decode(&body); err != nil {
		return err
	}
	*a = Approval{body: body}
	return nil
}

// Approvals is the list of approvals.
type Approvals []*Approval

// RootHash computes merkle root hash of backers.
func (as Approvals) RootHash() thor.Bytes32 {
	if len(as) == 0 {
		// optimized
		return emptyRoot
	}
	return trie.DeriveRoot(derivableApprovals(as))
}

// implements DerivableList.
type derivableApprovals Approvals

func (as derivableApprovals) Len() int {
	return len(as)
}
func (as derivableApprovals) GetRlp(i int) []byte {
	data, err := rlp.EncodeToBytes(as[i])
	if err != nil {
		panic(err)
	}
	return data
}

// FullApproval is the block approval with proposal.
type FullApproval struct {
	body struct {
		ProposalHash thor.Bytes32
		Approval     *Approval
	}
	cache struct {
		hash atomic.Value
	}
}

// NewFullApproval creates a new approval with proposal hash.
func NewFullApproval(proposalHash thor.Bytes32, a *Approval) *FullApproval {
	var full FullApproval
	full.body.ProposalHash = proposalHash
	full.body.Approval = a
	return &full
}

// ProposalHash returns the hash of the proposal.
func (a *FullApproval) ProposalHash() thor.Bytes32 {
	return a.body.ProposalHash
}

// Approval returns a copy of the approval.
func (a *FullApproval) Approval() *Approval {
	cpy := a.body.Approval
	return cpy
}

// Hash is the hash of RLP encoded full approval.
func (a *FullApproval) Hash() (hash thor.Bytes32) {
	if cached := a.cache.hash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { a.cache.hash.Store(hash) }()

	hw := thor.NewBlake2b()
	rlp.Encode(hw, a)
	hw.Sum(hash[:0])
	return
}

// EncodeRLP implements rlp.Encoder.
func (a *FullApproval) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &a.body)
}

// DecodeRLP implements rlp.Decoder.
func (a *FullApproval) DecodeRLP(s *rlp.Stream) error {
	var body struct {
		ProposalHash thor.Bytes32
		Approval     *Approval
	}

	if err := s.Decode(&body); err != nil {
		return err
	}
	*a = FullApproval{body: body}
	return nil
}
