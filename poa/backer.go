// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"bytes"
	"crypto/ecdsa"

	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/thor"
)

// IsBacker computes the VRF output regarding the input(alpha) and evalutes if the output(beta) meets the requirement of backer.
func IsBacker(privateKey *ecdsa.PrivateKey, alpha []byte) (result bool, proof []byte, err error) {
	vrf := ecvrf.NewSecp256k1Sha256Tai()
	beta, proof, err := vrf.Prove(privateKey, alpha)
	if err != nil {
		return
	}

	result = evaluateBeta(beta)
	return
}

// VerifyBacker verifies the given proof with public public key and evalutes if the output(beta) meets the requirement of backer.
func VerifyBacker(publicKey *ecdsa.PublicKey, alpha []byte, proof []byte) (bool, error) {
	vrf := ecvrf.NewSecp256k1Sha256Tai()

	beta, err := vrf.Verify(publicKey, alpha, proof)
	if err != nil {
		return false, err
	}
	return evaluateBeta(beta), nil
}

func evaluateBeta(beta []byte) bool {
	if c := bytes.Compare(beta, thor.BackerThreshold.Bytes()); c == -1 {
		return true
	}
	return false
}
