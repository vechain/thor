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

// TryApprove tries to compute the VRF output regarding the input(alpha) and the given private key
// then evalutes if the output(beta) meets the requirement of backer and return the proof.
func TryApprove(privateKey *ecdsa.PrivateKey, alpha []byte) (lucky bool, proof []byte, err error) {
	vrf := ecvrf.NewSecp256k1Sha256Tai()
	beta, proof, err := vrf.Prove(privateKey, alpha)
	if err != nil {
		return
	}

	lucky = EvaluateVRF(beta)
	return
}

// EvaluateVRF evalutes if the VRF output(beta) meets the requirement of backer.
func EvaluateVRF(beta []byte) bool {
	if c := bytes.Compare(beta, thor.BackerThreshold.Bytes()); c == -1 {
		return true
	}
	return false
}
