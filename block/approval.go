// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// Approval is the approval of a block
type Approval struct {
	body struct {
		PublicKey []byte
		Proof     []byte
	}
	cache struct {
		hash   atomic.Value
		signer atomic.Value
	}
}

// NewApproval creates a new approval
func NewApproval(pub, proof []byte) *Approval {
	var a Approval
	a.body.PublicKey = pub
	a.body.Proof = proof

	return &a
}

// Hash is the hash of RLP encoded approval
func (a *Approval) Hash() (hash thor.Bytes32) {
	if cached := a.cache.hash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { a.cache.hash.Store(hash) }()

	hw := thor.NewBlake2b()
	rlp.Encode(hw, a)
	hw.Sum(hash[:])
	return
}

// Signer computes the address from the public key
func (a *Approval) Signer() (signer thor.Address) {
	if cached := a.cache.signer.Load(); cached != nil {
		return cached.(thor.Address)
	}
	defer func() { a.cache.signer.Store(signer) }()

	signer = thor.Address(common.BytesToAddress(crypto.Keccak256(a.body.PublicKey[1:])[12:]))
	return
}

// EncodeRLP implements rlp.Encoder
func (a *Approval) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, a.body)
}

// DecodeRLP implements rlp.Decoder
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
