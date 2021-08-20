// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package chain

import (
	"encoding/binary"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// BlockSummary presents block summary.
type BlockSummary struct {
	Header    *block.Header
	IndexRoot thor.Bytes32
	Txs       []thor.Bytes32
	Size      uint64
	Extension extension
}

type extension struct {
	Beta []byte
}
type _extension extension

// EncodeRLP implements rlp.Encoder.
func (ex *extension) EncodeRLP(w io.Writer) error {
	// trim extension if beta is empty
	if len(ex.Beta) == 0 {
		return nil
	}
	return rlp.Encode(w, (*_extension)(ex))
}

// DecodeRLP implements rlp.Decoder.
func (ex *extension) DecodeRLP(s *rlp.Stream) error {
	var obj _extension
	if err := s.Decode(&obj); err != nil {
		// Error(end-of-list) means this field is not present, return default value
		// for backward compatibility
		if err == rlp.EOL {
			*ex = extension{
				nil,
			}
			return nil
		}
		return err
	}
	if len(obj.Beta) == 0 {
		return errors.New("rlp(BlockSummary): extension must be trimmed")
	}
	*ex = extension(obj)
	return nil
}

func (bs *BlockSummary) Beta() []byte {
	return bs.Extension.Beta
}

// the key for tx/receipt.
// it consists of: ( block id | infix | index )
type txKey [32 + 1 + 8]byte

func makeTxKey(blockID thor.Bytes32, infix byte) (k txKey) {
	copy(k[:], blockID[:])
	k[32] = infix
	return
}

func (k *txKey) SetIndex(i uint64) {
	binary.BigEndian.PutUint64(k[33:], i)
}
