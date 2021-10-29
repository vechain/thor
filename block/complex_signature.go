// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"
)

// ComplexSignature is the signature composed by[ECDSA(Secp256k1) Signature(65 bytes)+VRF Proof(81 bytes)]
type ComplexSignature []byte

// NewComplexSignature creates a new signature.
func NewComplexSignature(signature, proof []byte) (ComplexSignature, error) {
	if len(signature) != 65 {
		return nil, errors.New("invalid signature length, 65 bytes required")
	}
	if len(proof) != 81 {
		return nil, errors.New("invalid proof length, 81 bytes required")
	}

	var ms ComplexSignature
	ms = make([]byte, 0, ComplexSigSize)
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
