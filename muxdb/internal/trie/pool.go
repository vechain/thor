// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"sync"

	"github.com/vechain/thor/thor"
)

type buffer struct {
	b []byte
}

var (
	hasherPool = sync.Pool{
		New: func() interface{} {
			return thor.NewBlake2b()
		},
	}
	bufferPool = sync.Pool{
		New: func() interface{} {
			return &buffer{}
		},
	}
)
