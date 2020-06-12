// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"bytes"
	"errors"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/tx"
)

// Raw allows to partially decode components of a block.
type Raw []byte

// DecodeHeader decode only the header.
func (r Raw) DecodeHeader() (*Header, error) {
	content, _, err := rlp.SplitList(r)
	if err != nil {
		return nil, err
	}

	var header Header
	if err := rlp.Decode(bytes.NewReader(content), &header); err != nil {
		return nil, err
	}
	return &header, nil
}

// DecodeBody decode only the body.
func (r Raw) DecodeBody() (*Body, error) {
	var (
		raws             []rlp.RawValue
		txs              tx.Transactions
		backerSignatures BackerSignatures
	)

	if err := rlp.Decode(bytes.NewReader(r), &raws); err != nil {
		return nil, err
	}
	if len(raws) > 3 {
		return nil, errors.New("rlp:block body has too many fields")
	}
	if err := rlp.Decode(bytes.NewReader(raws[1]), &txs); err != nil {
		return nil, err
	}
	if len(raws) == 3 {
		if err := rlp.Decode(bytes.NewReader(raws[2]), &backerSignatures); err != nil {
			return nil, err
		}
	} else {
		backerSignatures = BackerSignatures(nil)
	}

	return &Body{txs, backerSignatures}, nil
}
