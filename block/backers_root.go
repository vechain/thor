// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

type backersRoot struct {
	Root              thor.Bytes32
	TotalBackersCount uint64
}

type _backersRoot backersRoot

// DecodeRLP implements rlp.Decoder.
func (br *backersRoot) DecodeRLP(s *rlp.Stream) error {
	k, _, _ := s.Kind()
	if k == rlp.List {
		var obj _backersRoot
		if err := s.Decode(&obj); err != nil {
			return err
		}
		*br = backersRoot(obj)
	} else {
		*br = backersRoot{
			emptyRoot,
			0,
		}
	}
	return nil
}
