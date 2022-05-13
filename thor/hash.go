// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"bytes"
	"hash"
	"io"
	"sync"

	"github.com/vechain/thor/blake2b"
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
func Blake2bFn(fn func(w io.Writer)) (s Bytes32) {
	w := hashBufferPool.Get().(*bytes.Buffer)
	fn(w)
	s = blake2b.Sum256(w.Bytes())
	w.Reset()
	hashBufferPool.Put(w)
	return
}

var hashBufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(nil)
	},
}
