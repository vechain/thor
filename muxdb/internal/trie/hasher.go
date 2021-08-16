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
	hash.Hash
	tmp []byte
}

var hasherPool = sync.Pool{
	New: func() interface{} {
		return &hasher{Hash: thor.NewBlake2b()}
	},
}
