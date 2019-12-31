// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type txsRootFeatures struct {
	Root     thor.Bytes32
	Features tx.Features // supported features
}

// new type to avoid recursion
type _txsRootFeatures txsRootFeatures

func (trf *txsRootFeatures) EncodeRLP(w io.Writer) error {
	if trf.Features == 0 {
		// backward compatible
		return rlp.Encode(w, &trf.Root)
	}

	return rlp.Encode(w, (*_txsRootFeatures)(trf))
}

func (trf *txsRootFeatures) DecodeRLP(s *rlp.Stream) error {
	kind, _, _ := s.Kind()
	if kind == rlp.List {
		var obj _txsRootFeatures
		if err := s.Decode(&obj); err != nil {
			return err
		}
		*trf = txsRootFeatures(obj)
	} else {
		var root thor.Bytes32
		if err := s.Decode(&root); err != nil {
			return err
		}
		*trf = txsRootFeatures{
			root,
			0,
		}
	}
	return nil
}
