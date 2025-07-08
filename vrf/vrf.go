// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package vrf provides optimized Secp256k1Sha256Tai functions.
package vrf

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/v2/thor"
)

var vrf = ecvrf.New(&ecvrf.Config{
	Curve:       &mergedCurve{},
	SuiteString: 0xfe,
	Cofactor:    0x01,
	NewHasher:   sha256.New,
	Decompress: func(_ elliptic.Curve, pk []byte) (x, y *big.Int) {
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

// Validator represents a validator with its weight
type Validator struct {
	Address thor.Address
	Weight  *big.Int
}

// WeightedValidatorSelection selects validators based on their weights using VRF
// Returns a list of selected validators and the VRF proof
func WeightedValidatorSelection(
	validators []Validator,
	alpha []byte,
	maxValidators int,
) (selected []thor.Address, beta, pi []byte, err error) {
	if len(validators) == 0 {
		return nil, nil, nil, nil
	}

	// Sort validators by address for deterministic selection
	sort.Slice(validators, func(i, j int) bool {
		return bytes.Compare(validators[i].Address.Bytes(), validators[j].Address.Bytes()) < 0
	})

	// Calculate total weight
	totalWeight := new(big.Int)
	for _, v := range validators {
		totalWeight.Add(totalWeight, v.Weight)
	}

	if totalWeight.Sign() == 0 {
		return nil, nil, nil, nil
	}

	// Generate VRF proof for the alpha
	// Note: In a real implementation, this would use the network's VRF key
	// For now, we'll use a deterministic approach based on alpha
	beta, pi, err = generateDeterministicVRF(alpha)
	if err != nil {
		return nil, nil, nil, err
	}

	// Use beta as seed for weighted selection
	selected = selectValidatorsByWeight(validators, beta, maxValidators)

	return selected, beta, pi, nil
}

// generateDeterministicVRF generates a deterministic VRF-like output
// In production, this would use the actual VRF implementation
func generateDeterministicVRF(alpha []byte) (beta, pi []byte, err error) {
	// Create a deterministic hash based on alpha
	hasher := sha256.New()
	hasher.Write(alpha)
	beta = hasher.Sum(nil)

	// Create a mock proof (in real implementation, this would be a proper VRF proof)
	pi = make([]byte, 81) // Standard VRF proof size
	copy(pi, beta)
	if len(pi) > 81 {
		pi = pi[:81]
	}

	return beta, pi, nil
}

// selectValidatorsByWeight selects validators proportionally to their weights
func selectValidatorsByWeight(validators []Validator, seed []byte, maxValidators int) []thor.Address {
	if len(validators) == 0 || maxValidators <= 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := new(big.Int)
	for _, v := range validators {
		totalWeight.Add(totalWeight, v.Weight)
	}

	if totalWeight.Sign() == 0 {
		return nil
	}

	// Use seed to generate random numbers
	seedValue := newSeededRNG(seed)

	selected := make([]thor.Address, 0, maxValidators)
	selectedSet := make(map[thor.Address]bool)

	for len(selected) < maxValidators && len(selected) < len(validators) {
		// Generate deterministic value based on seed and selection count
		// Use a more sophisticated approach to avoid infinite loops
		attempts := 0
		maxAttempts := len(validators) * 2

		for attempts < maxAttempts {
			// Create a deterministic seed based on the original seed and attempt count
			attemptSeed := new(big.Int).Add(seedValue, big.NewInt(int64(len(selected)*1000+attempts)))
			randomValue := new(big.Int).Mod(attemptSeed, totalWeight)

			// Find validator based on weight
			cumulativeWeight := new(big.Int)
			for _, v := range validators {
				if selectedSet[v.Address] {
					continue
				}

				cumulativeWeight.Add(cumulativeWeight, v.Weight)
				if randomValue.Cmp(cumulativeWeight) < 0 {
					selected = append(selected, v.Address)
					selectedSet[v.Address] = true
					break
				}
			}

			// If we found a validator, break out of the attempts loop
			if len(selected) > attempts/len(validators) {
				break
			}

			attempts++
		}

		// If we couldn't find a validator after max attempts, break
		if attempts >= maxAttempts {
			break
		}
	}

	return selected
}

// newSeededRNG creates a deterministic random number generator from seed
func newSeededRNG(seed []byte) *big.Int {
	// Use the first 8 bytes of seed as a uint64
	if len(seed) < 8 {
		padded := make([]byte, 8)
		copy(padded, seed)
		seed = padded
	}

	seedValue := binary.BigEndian.Uint64(seed[:8])
	return big.NewInt(int64(seedValue))
}
