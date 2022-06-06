// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package vrf provides optimized Secp256k1Sha256Tai functions.
package vrf

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/vechain/go-ecvrf"
)

var vrf = ecvrf.New(&ecvrf.Config{
	Curve:       &mergedCurve{},
	SuiteString: 0xfe,
	Cofactor:    0x01,
	NewHasher:   sha256.New,
	Decompress: func(c elliptic.Curve, pk []byte) (x, y *big.Int) {
		return secp256k1.DecompressPubkey(pk)
	},
})

// Prove constructs a VRF proof `pi` for the given input `alpha`,
// using the private key `sk`. The hash output is returned as `beta`.
func Prove(sk *ecdsa.PrivateKey, alpha []byte) (beta, pi []byte, err error) {
	return vrf.Prove(sk, alpha)
}

// Verify checks the proof `pi` of the message `alpha` against the given
// public key `pk`. The hash output is returned as `beta`.
func Verify(pk *ecdsa.PublicKey, alpha, pi []byte) (beta []byte, err error) {
	return vrf.Verify(pk, alpha, pi)
}
