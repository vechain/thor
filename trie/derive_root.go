// Copyright (c) 2018 The VeChainThor developers
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"github.com/qianbin/drlp"
	"github.com/vechain/thor/v2/thor"
)

// see "github.com/ethereum/go-ethereum/types/derive_sha.go"

type DerivableList interface {
	Len() int
	GetRlp(i int) []byte
}

func DeriveRoot(list DerivableList) thor.Bytes32 {
	var (
		trie Trie
		key  []byte
	)

	for i := 0; i < list.Len(); i++ {
		key = drlp.AppendUint(key[:0], uint64(i))
		trie.Update(key, list.GetRlp(i), nil)
	}

	return trie.Hash()
}
