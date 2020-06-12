// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/tx"
)

// Block is an immutable block type.
type Block struct {
	header           *Header
	txs              tx.Transactions
	backerSignatures BackerSignatures
	cache            struct {
		size atomic.Value
	}
}

// Body defines body of a block.
type Body struct {
	Txs              tx.Transactions
	BackerSignatures BackerSignatures
}

// Compose compose a block with all needed components
// Note: This method is usually to recover a block by its portions, and the TxsRoot is not verified.
// To build up a block, use a Builder.
func Compose(header *Header, txs tx.Transactions, backerSignatures BackerSignatures) *Block {
	return &Block{
		header:           header,
		txs:              append(tx.Transactions(nil), txs...),
		backerSignatures: append(BackerSignatures(nil), backerSignatures...),
	}
}

// WithSignature create a new block object with signature set.
func (b *Block) WithSignature(sig []byte) *Block {
	return &Block{
		header:           b.header.withSignature(sig),
		txs:              b.txs,
		backerSignatures: b.backerSignatures,
	}
}

// Header returns the block header.
func (b *Block) Header() *Header {
	return b.header
}

// Transactions returns a copy of transactions.
func (b *Block) Transactions() tx.Transactions {
	return append(tx.Transactions(nil), b.txs...)
}

// BackerSignatures returns a copy of backer signature list.
func (b *Block) BackerSignatures() BackerSignatures {
	return append(BackerSignatures(nil), b.backerSignatures...)
}

// Body returns body of a block.
func (b *Block) Body() *Body {
	return &Body{
		append(tx.Transactions(nil), b.txs...),
		append(BackerSignatures(nil), b.backerSignatures...),
	}
}

// EncodeRLP implements rlp.Encoder.
func (b *Block) EncodeRLP(w io.Writer) error {
	if b.backerSignatures == nil {
		// backward compatible
		return rlp.Encode(w, []interface{}{
			b.header,
			b.txs,
		})
	}

	return rlp.Encode(w, []interface{}{
		b.header,
		b.txs,
		b.backerSignatures,
	})
}

// DecodeRLP implements rlp.Decoder.
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()

	var (
		raws             []rlp.RawValue
		header           Header
		txs              tx.Transactions
		backerSignatures BackerSignatures
	)

	if err := s.Decode(&raws); err != nil {
		return err
	}
	if len(raws) > 3 {
		return errors.New("rlp:block body has too many fields")
	}
	if err := rlp.Decode(bytes.NewReader(raws[0]), &header); err != nil {
		return err
	}
	if err := rlp.Decode(bytes.NewReader(raws[1]), &txs); err != nil {
		return err
	}
	if len(raws) == 3 {
		if err := rlp.Decode(bytes.NewReader(raws[2]), &backerSignatures); err != nil {
			return err
		}
	} else {
		backerSignatures = BackerSignatures(nil)
	}

	*b = Block{
		header:           &header,
		txs:              txs,
		backerSignatures: backerSignatures,
	}
	b.cache.size.Store(metric.StorageSize(rlp.ListSize(size)))
	return nil
}

// Size returns block size in bytes.
func (b *Block) Size() metric.StorageSize {
	if cached := b.cache.size.Load(); cached != nil {
		return cached.(metric.StorageSize)
	}
	var size metric.StorageSize
	rlp.Encode(&size, b)
	b.cache.size.Store(size)
	return size
}

func (b *Block) String() string {
	return fmt.Sprintf(`Block(%v)
%v
Transactions: %v`, b.Size(), b.header, b.txs)
}
