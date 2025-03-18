// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"hash"
	"io"
	"sync"

	"github.com/ethereum/go-ethereum/crypto/blake2b"
	"golang.org/x/crypto/sha3"
)

// NewBlake2b return blake2b-256 hash.
func NewBlake2b() hash.Hash {
	hash, _ := blake2b.New256(nil)
	return hash
}

// Blake2b computes blake2b-256 checksum for given data.
func Blake2b(data ...[]byte) Bytes32 {
	if len(data) == 1 {
		// the quick version
		return blake2b.Sum256(data[0])
	} else {
		return Blake2bFn(func(w io.Writer) {
			for _, b := range data {
				w.Write(b)
			}
		})
	}
}

// Blake2bFn computes blake2b-256 checksum for the provided writer.
func Blake2bFn(fn func(w io.Writer)) (h Bytes32) {
	w := blake2bStatePool.Get().(*blake2bState)
	fn(w)
	w.Sum(w.b32[:0])
	h = w.b32 // to avoid 1 alloc
	w.Reset()
	blake2bStatePool.Put(w)
	return
}

type blake2bState struct {
	hash.Hash
	b32 Bytes32
}

var blake2bStatePool = sync.Pool{
	New: func() any {
		return &blake2bState{
			Hash: NewBlake2b(),
		}
	},
}

// keccakState wraps sha3.state. In addition to the usual hash methods, it also supports
// Read to get a variable amount of data from the hash state. Read is faster than Sum
// because it doesn't copy the internal state, but also modifies the internal state.
type keccakState interface {
	hash.Hash
	Read([]byte) (int, error)
}

type keccak256 struct {
	state keccakState
	b32   Bytes32
}

var keccak256Pool = sync.Pool{
	New: func() any {
		return &keccak256{
			state: sha3.NewLegacyKeccak256().(keccakState),
		}
	},
}

func Keccak256(data ...[]byte) (h Bytes32) {
	hasher := keccak256Pool.Get().(*keccak256)

	for _, b := range data {
		hasher.state.Write(b)
	}
	hasher.state.Read(hasher.b32[:])
	h = hasher.b32

	hasher.state.Reset()
	keccak256Pool.Put(hasher)
	return
}
