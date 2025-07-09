// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vrf_test

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/vrf"
)

// Case Testing cases structure.
type Case struct {
	Sk    string `json:"sk"`
	Pk    string `json:"pk"`
	Alpha string `json:"alpha"`
	Pi    string `json:"pi"`
	Beta  string `json:"beta"`
}

func readCases(fileName string) ([]Case, error) {
	jsonFile, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer jsonFile.Close()

	byteValue, err2 := io.ReadAll(jsonFile)
	if err2 != nil {
		return nil, err2
	}

	var cases = make([]Case, 0)
	err3 := json.Unmarshal(byteValue, &cases)
	if err3 != nil {
		return cases, err3
	}

	return cases, nil
}

func Test_Secp256K1Sha256Tai_vrf_Prove(t *testing.T) {
	// Know Correct cases.
	var cases, _ = readCases("./secp256_k1_sha256_tai.json")

	type Test struct {
		name     string
		sk       *ecdsa.PrivateKey
		alpha    []byte
		wantBeta []byte
		wantPi   []byte
		wantErr  bool
	}

	tests := []Test{}
	for _, c := range cases {
		skBytes, _ := hex.DecodeString(c.Sk)
		sk := crypto.ToECDSAUnsafe(skBytes)

		alpha, _ := hex.DecodeString(c.Alpha)
		wantBeta, _ := hex.DecodeString(c.Beta)
		wantPi, _ := hex.DecodeString(c.Pi)

		tests = append(tests, Test{
			c.Sk,
			sk,
			alpha,
			wantBeta,
			wantPi,
			false,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBeta, gotPi, err := vrf.Prove(tt.sk, tt.alpha)
			if (err != nil) != tt.wantErr {
				t.Errorf("vrf.Prove() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotBeta, tt.wantBeta) {
				t.Errorf("vrf.Prove() gotBeta = %v, want %v", hex.EncodeToString(gotBeta), hex.EncodeToString(tt.wantBeta))
			}
			if !reflect.DeepEqual(gotPi, tt.wantPi) {
				t.Errorf("vrf.Prove() gotPi = %v, want %v", hex.EncodeToString(gotPi), hex.EncodeToString(tt.wantPi))
			}
		})
	}
}

func Test_Secp256K1Sha256Tai_vrf_Verify(t *testing.T) {
	// Know Correct cases.
	var cases, _ = readCases("./secp256_k1_sha256_tai.json")

	type Test struct {
		name     string
		pk       *ecdsa.PublicKey
		alpha    []byte
		pi       []byte
		wantBeta []byte
		wantErr  bool
	}

	tests := []Test{}
	for _, c := range cases {
		skBytes, _ := hex.DecodeString(c.Sk)
		sk := crypto.ToECDSAUnsafe(skBytes)

		pk := &sk.PublicKey

		alpha, _ := hex.DecodeString(c.Alpha)

		wantPi, _ := hex.DecodeString(c.Pi)

		wantBeta, _ := hex.DecodeString(c.Beta)

		tests = append(tests, Test{
			c.Alpha,
			pk,
			alpha,
			wantPi,
			wantBeta,
			false,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBeta, err := vrf.Verify(tt.pk, tt.alpha, tt.pi)
			if (err != nil) != tt.wantErr {
				t.Errorf("vrf.Verify() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotBeta, tt.wantBeta) {
				t.Errorf("vrf.Verify() = %v, want %v", gotBeta, tt.wantBeta)
			}
		})
	}
}

func Test_Secp256K1Sha256Tai_vrf_Verify_bad_message(t *testing.T) {
	type Test struct {
		name     string
		pk       *ecdsa.PublicKey
		alpha    []byte
		pi       []byte
		wantBeta []byte
		wantErr  bool
	}

	// sk
	skBytes, _ := hex.DecodeString("c9afa9d845ba75166b5c215767b1d6934e50c3db36e89b127b8a622b120f6721")
	sk := crypto.ToECDSAUnsafe(skBytes)

	// pk
	pk := &sk.PublicKey

	// correct alpha
	// alpha, _ := hex.DecodeString("73616d706c65")
	wrongAlpha := []byte("Hello VeChain")
	// pi
	wantPi, _ := hex.DecodeString("031f4dbca087a1972d04a07a779b7df1caa99e0f5db2aa21f3aecc4f9e10e85d08748c9fbe6b95d17359707bfb8e8ab0c93ba0c515333adcb8b64f372c535e115ccf66ebf5abe6fadb01b5efb37c0a0ec9")

	// beta
	wantBeta, _ := hex.DecodeString("612065e309e937ef46c2ef04d5886b9c6efd2991ac484ec64a9b014366fc5d81")

	// test case
	tt := Test{
		"bad_message",
		pk,
		wrongAlpha,
		wantPi,
		wantBeta,
		true,
	}

	t.Run(tt.name, func(t *testing.T) {
		_, err := vrf.Verify(tt.pk, tt.alpha, tt.pi)
		if (err != nil) != tt.wantErr {
			t.Errorf("vrf.Verify() error = %v, wantErr %v", err, tt.wantErr)
			return
		}
	})
}

func BenchmarkVRF(b *testing.B) {
	b.Run("vrf-proving", func(b *testing.B) {
		sk, _ := crypto.GenerateKey()
		alpha := []byte("Hello VeChain")

		b.ResetTimer()
		for b.Loop() {
			_, _, err := vrf.Prove(sk, alpha)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("vrf-verifying", func(b *testing.B) {
		sk, _ := crypto.GenerateKey()
		alpha := []byte("Hello VeChain")

		_, pi, _ := vrf.Prove(sk, alpha)
		b.ResetTimer()
		for b.Loop() {
			_, err := vrf.Verify(&sk.PublicKey, alpha, pi)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestWeightedValidatorSelectionMaxValidators(t *testing.T) {
	validators := []vrf.Validator{
		{
			Address: thor.BytesToAddress([]byte("validator1")),
			Weight:  big.NewInt(100),
		},
		{
			Address: thor.BytesToAddress([]byte("validator2")),
			Weight:  big.NewInt(200),
		},
	}

	// Create private keys for validators
	privateKey1, _ := crypto.GenerateKey()
	privateKey2, _ := crypto.GenerateKey()

	validatorPrivateKeys := map[thor.Address]*ecdsa.PrivateKey{
		validators[0].Address: privateKey1,
		validators[1].Address: privateKey2,
	}

	alpha := []byte("test alpha seed")
	maxValidators := 5 // More than available validators

	selected, _, _, err := vrf.WeightedValidatorSelection(validators, alpha, maxValidators, validatorPrivateKeys)
	assert.NoError(t, err)
	assert.Len(t, selected, 2, "should select all available validators")
}

func TestWeightedValidatorSelectionWithProofs(t *testing.T) {
	// Create test validators with different weights
	validators := []vrf.Validator{
		{
			Address: thor.BytesToAddress([]byte("validator1")),
			Weight:  big.NewInt(100),
		},
		{
			Address: thor.BytesToAddress([]byte("validator2")),
			Weight:  big.NewInt(200),
		},
		{
			Address: thor.BytesToAddress([]byte("validator3")),
			Weight:  big.NewInt(150),
		},
	}

	// Create simulated VRF proofs
	validatorProofs := map[thor.Address][]byte{
		validators[0].Address: []byte("proof1"),
		validators[1].Address: []byte("proof2"),
		validators[2].Address: []byte("proof3"),
	}

	alpha := []byte("test alpha seed")
	maxValidators := 2

	selected, beta, pi, err := vrf.WeightedValidatorSelectionWithProofs(validators, alpha, maxValidators, validatorProofs)
	assert.NoError(t, err)
	assert.Len(t, selected, 2, "should select exactly maxValidators")
	assert.NotNil(t, beta, "should return beta")
	assert.NotNil(t, pi, "should return pi")

	// Verify that selected validators are from the input list
	for _, addr := range selected {
		found := false
		for _, v := range validators {
			if v.Address == addr {
				found = true
				break
			}
		}
		assert.True(t, found, "selected validator should be from input list")
	}

	// Test with empty validators
	selected, beta, pi, err = vrf.WeightedValidatorSelectionWithProofs([]vrf.Validator{}, alpha, maxValidators, validatorProofs)
	assert.NoError(t, err)
	assert.Nil(t, selected)
	assert.Nil(t, beta)
	assert.Nil(t, pi)

	// Test with no proofs - should return error
	selected, beta, pi, err = vrf.WeightedValidatorSelectionWithProofs(validators, alpha, maxValidators, nil)
	assert.Error(t, err, "should return error when no proofs available")
	assert.Contains(t, err.Error(), "no valid VRF proofs available")
}
