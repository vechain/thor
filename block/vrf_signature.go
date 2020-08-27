// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

var (
	emptyRoot = trie.DeriveRoot(&derivableVRFSignatures{})
)

// VRFSignature is the backer's signature of a block declaration by VRF.
// Composed by [ Compressed Public Key(33bytes) + Proof(81bytes) ]
type VRFSignature struct {
	body  []byte
	cache struct {
		signer atomic.Value
	}
}

// NewVRFSignature creates a new signature.
func NewVRFSignature(pub, proof []byte) *VRFSignature {
	var vs VRFSignature
	vs.body = append(vs.body, pub...)
	vs.body = append(vs.body, proof...)

	return &vs
}

// Bytes returns the content in byte slice.
func (vs *VRFSignature) Bytes() []byte {
	return append([]byte(nil), vs.body...)
}

// Validate validates the proof and returns the VRF output.
func (vs *VRFSignature) Validate(alpha []byte) (beta []byte, err error) {
	if len(vs.body) != 81+33 {
		return nil, errors.New("invalid VRF signature length, 114 bytes needed")
	}

	pub := make([]byte, 33)
	proof := make([]byte, 81)

	copy(pub[:], vs.body[:])
	copy(proof[:], vs.body[33:])

	vrf := ecvrf.NewSecp256k1Sha256Tai()
	pubkey, err := crypto.DecompressPubkey(pub)
	if err != nil {
		return nil, err
	}
	beta, err = vrf.Verify(pubkey, alpha, proof)
	return
}

// Signer computes the address from the public key.
func (vs *VRFSignature) Signer() (signer thor.Address, err error) {
	if cached := vs.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() { vs.cache.signer.Store(signer) }()

	pub := make([]byte, 33)
	copy(pub[:], vs.body[:])

	pubkey, err := crypto.DecompressPubkey(pub)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pubkey))
	return
}

// EncodeRLP implements rlp.Encoder.
func (vs *VRFSignature) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &vs.body)
}

// DecodeRLP implements rlp.Decoder.
func (vs *VRFSignature) DecodeRLP(s *rlp.Stream) error {
	var body []byte

	if err := s.Decode(&body); err != nil {
		return err
	}
	*vs = VRFSignature{body: body}
	return nil
}

// VRFSignatures is the list of VRF signature.
type VRFSignatures []*VRFSignature

// RootHash computes merkle root hash of VRFSignatures.
func (vss VRFSignatures) RootHash() thor.Bytes32 {
	if len(vss) == 0 {
		// optimized
		return emptyRoot
	}
	return trie.DeriveRoot(derivableVRFSignatures(vss))
}

// implements DerivableList.
type derivableVRFSignatures VRFSignatures

func (d derivableVRFSignatures) Len() int {
	return len(d)
}
func (d derivableVRFSignatures) GetRlp(i int) []byte {
	data, err := rlp.EncodeToBytes(d[i])
	if err != nil {
		panic(err)
	}
	return data
}
