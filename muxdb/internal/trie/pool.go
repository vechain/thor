// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"hash"
	"sync"

	"github.com/vechain/thor/thor"
)

type hasher struct {
	a   interface{}
	buf []byte
}

func (h *hasher) Hash(in []byte) []byte {
	var a hash.Hash
	a, ok := h.a.(hash.Hash)
	if ok {
		a.Reset()
	} else {
		a = thor.NewBlake2b()
		h.a = a
	}

	a.Write(in)
	h.buf = a.Sum(h.buf[:0])
	return h.buf
}

var hasherPool = sync.Pool{
	New: func() interface{} {
		return &hasher{}
	},
}
