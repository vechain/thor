// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

var emptyRoot = trie.DeriveRoot(&derivableTxs{})

// Transactions a slice of transactions.
type Transactions []*Transaction

// RootHash computes merkle root hash of transactions.
func (txs Transactions) RootHash() thor.Bytes32 {
	if len(txs) == 0 {
		// optimized
		return emptyRoot
	}
	return trie.DeriveRoot(derivableTxs(txs))
}

// implements types.DerivableList
type derivableTxs Transactions

func (txs derivableTxs) Len() int {
	return len(txs)
}

func (txs derivableTxs) EncodeIndex(i int) []byte {
	data, err := txs[i].MarshalBinary()
	if err != nil {
		panic(err)
	}
	return data
}
