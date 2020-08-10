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
	emptyRoot = trie.DeriveRoot(&derivableBackerSignatures{})
)

// BackerSignature is the backer's signature of a block proposal by VRF.
// Composed by [ Compressed Public Key(33bytes) + Proof(81bytes) ]
type BackerSignature struct {
	body  []byte
	cache struct {
		hash   atomic.Value
		signer atomic.Value
		beta   atomic.Value
	}
}

// NewBackerSignature creates a new backer signature.
func NewBackerSignature(pub, proof []byte) *BackerSignature {
	var bs BackerSignature
	bs.body = append(bs.body, pub...)
	bs.body = append(bs.body, proof...)

	return &bs
}

// Hash is the hash of backer signature.
func (bs *BackerSignature) Hash() (hash thor.Bytes32) {
	if cached := bs.cache.hash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { bs.cache.hash.Store(hash) }()

	hash = thor.Blake2b(bs.body)
	return
}

// Validate validates backer's proof and returns the VRF output.
func (bs *BackerSignature) Validate(alpha []byte) (beta []byte, err error) {
	if cached := bs.cache.beta.Load(); cached != nil {
		return cached.([]byte), nil
	}
	defer func() { bs.cache.beta.Store(beta) }()

	if len(bs.body) != 81+33 {
		return nil, errors.New("invalid backer signature length, 114 bytes needed")
	}

	pub := make([]byte, 33)
	proof := make([]byte, 81)

	copy(pub[:], bs.body[:])
	copy(proof[:], bs.body[33:])

	vrf := ecvrf.NewSecp256k1Sha256Tai()
	pubkey, err := crypto.DecompressPubkey(pub)
	if err != nil {
		return nil, err
	}
	beta, err = vrf.Verify(pubkey, alpha, proof)
	return
}

// Signer computes the address from the public key.
func (bs *BackerSignature) Signer() (signer thor.Address, err error) {
	if cached := bs.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() { bs.cache.signer.Store(signer) }()

	pub := make([]byte, 33)
	copy(pub[:], bs.body[:])

	pubkey, err := crypto.DecompressPubkey(pub)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pubkey))
	return
}

// EncodeRLP implements rlp.Encoder.
func (bs *BackerSignature) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &bs.body)
}

// DecodeRLP implements rlp.Decoder.
func (bs *BackerSignature) DecodeRLP(s *rlp.Stream) error {
	var body []byte

	if err := s.Decode(&body); err != nil {
		return err
	}
	*bs = BackerSignature{body: body}
	return nil
}

// BackerSignatures is the list of backer signature.
type BackerSignatures []*BackerSignature

// RootHash computes merkle root hash of backers.
func (bss BackerSignatures) RootHash() thor.Bytes32 {
	if len(bss) == 0 {
		// optimized
		return emptyRoot
	}
	return trie.DeriveRoot(derivableBackerSignatures(bss))
}

// implements DerivableList.
type derivableBackerSignatures BackerSignatures

func (d derivableBackerSignatures) Len() int {
	return len(d)
}
func (d derivableBackerSignatures) GetRlp(i int) []byte {
	data, err := rlp.EncodeToBytes(d[i])
	if err != nil {
		panic(err)
	}
	return data
}
