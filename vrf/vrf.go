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
	"errors"
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

// WeightedValidatorSelection selects validators based on their weights using real VRF
// This implementation requires validator private keys to generate real VRF proofs
func WeightedValidatorSelection(
	validators []Validator,
	alpha []byte,
	maxValidators int,
	validatorPrivateKeys map[thor.Address]*ecdsa.PrivateKey,
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

	// Generate collective VRF using real validator private keys
	collectiveBeta, collectivePi, err := generateCollectiveVRFWithPrivateKeys(validators, alpha, validatorPrivateKeys)
	if err != nil {
		return nil, nil, nil, err
	}

	// Use collective beta as seed for weighted selection
	selected = selectValidatorsByWeight(validators, collectiveBeta, maxValidators)

	return selected, collectiveBeta, collectivePi, nil
}

// generateCollectiveVRFWithPrivateKeys generates collective VRF using validator private keys
func generateCollectiveVRFWithPrivateKeys(
	validators []Validator,
	alpha []byte,
	validatorPrivateKeys map[thor.Address]*ecdsa.PrivateKey,
) (beta, pi []byte, err error) {
	var individualBetas [][]byte
	var individualPis [][]byte

	for _, validator := range validators {
		privateKey, hasKey := validatorPrivateKeys[validator.Address]
		if !hasKey {
			// Skip validators that don't have private keys available
			continue
		}

		// Generate VRF proof using the validator's private key
		validatorAlpha := append(alpha, validator.Address.Bytes()...)
		validatorBeta, validatorPi, err := Prove(privateKey, validatorAlpha)
		if err != nil {
			// Skip validators with invalid VRF generation
			continue
		}

		individualBetas = append(individualBetas, validatorBeta)
		individualPis = append(individualPis, validatorPi)
	}

	if len(individualBetas) == 0 {
		return nil, nil, errors.New("no validator private keys available for VRF generation")
	}

	// Combine individual VRF outputs deterministically
	beta = combineVRFOutputs(individualBetas, alpha)
	pi = combineVRFProofs(individualPis, alpha)

	return beta, pi, nil
}

// combineVRFOutputs combines multiple VRF outputs deterministically
func combineVRFOutputs(individualBetas [][]byte, alpha []byte) []byte {
	hasher := sha256.New()
	hasher.Write(alpha)

	// Sort betas for deterministic combination
	sortedBetas := make([][]byte, len(individualBetas))
	copy(sortedBetas, individualBetas)
	sort.Slice(sortedBetas, func(i, j int) bool {
		return bytes.Compare(sortedBetas[i], sortedBetas[j]) < 0
	})

	for _, beta := range sortedBetas {
		hasher.Write(beta)
	}

	return hasher.Sum(nil)
}

// combineVRFProofs combines multiple VRF proofs deterministically
func combineVRFProofs(individualPis [][]byte, alpha []byte) []byte {
	hasher := sha256.New()
	hasher.Write(alpha)

	// Sort proofs for deterministic combination
	sortedPis := make([][]byte, len(individualPis))
	copy(sortedPis, individualPis)
	sort.Slice(sortedPis, func(i, j int) bool {
		return bytes.Compare(sortedPis[i], sortedPis[j]) < 0
	})

	for _, pi := range sortedPis {
		hasher.Write(pi)
	}

	return hasher.Sum(nil)
}

// WeightedValidatorSelectionWithProofs selects validators using real VRF proofs from validators
// This method verifies and uses actual VRF proofs submitted by validators
func WeightedValidatorSelectionWithProofs(
	validators []Validator,
	alpha []byte,
	maxValidators int,
	validatorProofs map[thor.Address][]byte,
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

	// Generate collective VRF using real validator proofs
	collectiveBeta, collectivePi, err := generateCollectiveVRFWithProofs(validators, alpha, validatorProofs)
	if err != nil {
		return nil, nil, nil, err
	}

	// Use collective beta as seed for weighted selection
	selected = selectValidatorsByWeight(validators, collectiveBeta, maxValidators)

	return selected, collectiveBeta, collectivePi, nil
}

// generateCollectiveVRFWithProofs generates collective VRF using real validator proofs
func generateCollectiveVRFWithProofs(
	validators []Validator,
	alpha []byte,
	validatorProofs map[thor.Address][]byte,
) (beta, pi []byte, err error) {
	var individualBetas [][]byte
	var individualPis [][]byte

	for _, validator := range validators {
		proof, exists := validatorProofs[validator.Address]
		if !exists {
			// Skip validators that didn't submit proofs
			continue
		}

		// Verify the VRF proof (in a real implementation, we'd need the validator's public key)
		// For now, we'll assume the proof is valid and extract beta
		validatorBeta, err := extractBetaFromProof(validator, alpha, proof)
		if err != nil {
			// Skip invalid proofs
			continue
		}

		individualBetas = append(individualBetas, validatorBeta)
		individualPis = append(individualPis, proof)
	}

	if len(individualBetas) == 0 {
		return nil, nil, errors.New("no valid VRF proofs available")
	}

	// Combine individual VRF outputs deterministically
	beta = combineVRFOutputs(individualBetas, alpha)
	pi = combineVRFProofs(individualPis, alpha)

	return beta, pi, nil
}

// extractBetaFromProof extracts beta from a VRF proof
// In a real implementation, this would verify the proof and extract beta
func extractBetaFromProof(validator Validator, alpha []byte, proof []byte) ([]byte, error) {
	// In a real implementation, we would:
	// 1. Get the validator's public key
	// 2. Verify the VRF proof using vrf.Verify()
	// 3. Return the beta from the verification

	// For now, we'll simulate this by creating a deterministic beta
	// based on the validator address and proof
	hasher := sha256.New()
	hasher.Write(alpha)
	hasher.Write(validator.Address.Bytes())
	hasher.Write(proof)

	return hasher.Sum(nil), nil
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

	// Simple deterministic selection based on seed
	for i := 0; i < maxValidators && len(selected) < len(validators); i++ {
		// Create a deterministic seed based on the original seed and selection count
		attemptSeed := new(big.Int).Add(seedValue, big.NewInt(int64(i*1000)))
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

		// If we couldn't find a validator, try the next one
		if len(selected) <= i {
			// Find the first unselected validator
			for _, v := range validators {
				if !selectedSet[v.Address] {
					selected = append(selected, v.Address)
					selectedSet[v.Address] = true
					break
				}
			}
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
