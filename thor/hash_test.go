// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor_test

import (
	"hash"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/thor"
	"golang.org/x/crypto/sha3"
)

func BenchmarkHash(b *testing.B) {
	data := make([]byte, 10)

	rand.New(rand.NewSource(1)).Read(data) //#nosec G404

	b.Run("keccak", func(b *testing.B) {
		type keccakState interface {
			hash.Hash
			Read([]byte) (int, error)
		}

		k := sha3.NewLegacyKeccak256().(keccakState)
		var b32 thor.Bytes32
		for b.Loop() {
			k.Write(data)
			k.Read(b32[:])
			k.Reset()
		}
	})

	b.Run("blake2b", func(b *testing.B) {
		for b.Loop() {
			thor.Blake2b(data)
		}
	})
}

func BenchmarkBlake2b(b *testing.B) {
	data := make([]byte, 100)
	rand.New(rand.NewSource(1)).Read(data) //#nosec G404
	b.Run("Blake2b", func(b *testing.B) {
		for b.Loop() {
			thor.Blake2b(data).Bytes()
		}
	})

	b.Run("BlakeFn", func(b *testing.B) {
		for b.Loop() {
			thor.Blake2bFn(func(w io.Writer) {
				w.Write(data)
			})
		}
	})
}

func TestNewBlake2b(t *testing.T) {
	hasher := thor.NewBlake2b()
	if hasher == nil {
		t.Error("NewBlake2b returned nil")
	}

	testString := "VeChainThor"
	hasher.Write([]byte(testString))
	sum := hasher.Sum(nil)
	if len(sum) != 32 {
		t.Errorf("Expected BLAKE2b-256 hash length of 32, got %d", len(sum))
	}
}

func TestBlake2b(t *testing.T) {
	singleData := []byte("data")
	multipleData := [][]byte{[]byte("multi"), []byte("ple"), []byte("data")}

	// Single slice of data
	singleHash := thor.Blake2b(singleData)
	if len(singleHash) != 32 {
		t.Errorf("Expected hash length of 32, got %d", len(singleHash))
	}

	// Multiple slices of data
	multiHash := thor.Blake2b(multipleData...)
	if len(multiHash) != 32 {
		t.Errorf("Expected hash length of 32, got %d", len(multiHash))
	}

	// Check if different data results in different hashes
	if singleHash == multiHash {
		t.Error("Expected different hashes for different data")
	}
}

func TestBlake2bFn(t *testing.T) {
	h := thor.Blake2bFn(func(w io.Writer) {
		w.Write([]byte("custom writer"))
	})

	assert.Equal(t, thor.Blake2b([]byte("custom writer")), h)
}

func TestKeccak256(t *testing.T) {
	singleData := []byte("data")
	multipleData := [][]byte{[]byte("multi"), []byte("ple"), []byte("data")}

	// Single slice of data
	singleHash := thor.Keccak256(singleData)
	if len(singleHash) != 32 {
		t.Errorf("Expected hash length of 32, got %d", len(singleHash))
	}

	// Multiple slices of data
	multiHash := thor.Keccak256(multipleData...)
	if len(multiHash) != 32 {
		t.Errorf("Expected hash length of 32, got %d", len(multiHash))
	}

	// Check if different data results in different hashes
	if singleHash == multiHash {
		t.Error("Expected different hashes for different data")
	}
}
