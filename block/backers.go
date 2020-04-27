// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

var (
	emptyRoot = trie.DeriveRoot(&derivableBackers{})
)

// Backers is the list of approvals
type Backers []*Approval

type _backers Backers

// DecodeRLP implements rlp.Decoder
func (bs *Backers) DecodeRLP(s *rlp.Stream) error {
	k, _, _ := s.Kind()
	if k == rlp.List {
		var obj _backers
		if err := s.Decode(&obj); err != nil {
			return err
		}
		*bs = Backers(obj)
	} else {
		*bs = Backers{nil}
	}
	return nil
}

// RootHash computes merkle root hash of backers
func (bs Backers) RootHash() thor.Bytes32 {
	if len(bs) == 0 {
		// optimized
		return emptyRoot
	}
	return trie.DeriveRoot(derivableBackers(bs))
}

// implements DerivableList
type derivableBackers Backers

func (bs derivableBackers) Len() int {
	return len(bs)
}
func (bs derivableBackers) GetRlp(i int) []byte {
	data, err := rlp.EncodeToBytes(bs[i])
	if err != nil {
		panic(err)
	}
	return data
}
