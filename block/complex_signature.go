// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

var (
	emptyRoot = trie.DeriveRoot(&derivableComplexSignatures{})
)

// ComplexSignature is the signature from committee member.
// Composed by [ECDSA(Secp256k1) Signature(65 bytes)+VRF Proof(81 bytes)]
type ComplexSignature []byte

// NewComplexSignature creates a new signature.
func NewComplexSignature(proof, signature []byte) (ComplexSignature, error) {
	if len(proof) != 81 {
		return nil, errors.New("invalid proof length, 81 bytes required")
	}
	if len(signature) != 65 {
		return nil, errors.New("invalid signature length, 65 bytes required")
	}

	var ms ComplexSignature
	ms = make([]byte, 0, 146)
	ms = append(ms, signature...)
	ms = append(ms, proof...)

	return ms, nil
}

// Signature returns the ECDSA signature.
func (ms ComplexSignature) Signature() []byte {
	return ms[:65]
}

// Proof returns the VRF proof.
func (ms ComplexSignature) Proof() []byte {
	return ms[65:]
}

// DecodeRLP implements rlp.Decoder.
func (ms *ComplexSignature) DecodeRLP(s *rlp.Stream) error {
	var body []byte

	if err := s.Decode(&body); err != nil {
		return err
	} else if len(body) != 146 {
		return errors.New("rlp(complex signature): invalid length, want 146bytes")
	}

	*ms = body
	return nil
}

// ComplexSignatures is the list of VRF signature.
type ComplexSignatures []ComplexSignature

// RootHash computes merkle root hash of ComplexSignatures.
func (mss ComplexSignatures) RootHash() thor.Bytes32 {
	if len(mss) == 0 {
		// optimized
		return emptyRoot
	}
	return trie.DeriveRoot(derivableComplexSignatures(mss))
}

// implements DerivableList.
type derivableComplexSignatures ComplexSignatures

func (d derivableComplexSignatures) Len() int {
	return len(d)
}
func (d derivableComplexSignatures) GetRlp(i int) []byte {
	data, err := rlp.EncodeToBytes(d[i])
	if err != nil {
		panic(err)
	}
	return data
}
