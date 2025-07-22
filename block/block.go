// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"fmt"
	"io"
	"sync/atomic"

	"slices"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
	ComplexSigSize = 81 + 65
)

// Block is an immutable block type.
type Block struct {
	header    *Header
	txs       tx.Transactions
	evidences *[][]Header
	cache     struct {
		size atomic.Uint64
	}
}

// Body defines body of a block.
type Body struct {
	Txs       tx.Transactions
	Evidences *[][]Header
}

// Compose compose a block with all needed components
// Note: This method is usually to recover a block by its portions, and the TxsRoot is not verified.
// To build up a block, use a Builder.
func Compose(header *Header, txs tx.Transactions) *Block {
	return &Block{
		header: header,
		txs:    slices.Clone(txs),
	}
}

// WithSignature create a new block object with signature set.
func (b *Block) WithSignature(sig []byte) *Block {
	return &Block{
		header: b.header.withSignature(sig),
		txs:    b.txs,
	}
}

// Header returns the block header.
func (b *Block) Header() *Header {
	return b.header
}

// Transactions returns a copy of transactions.
func (b *Block) Transactions() tx.Transactions {
	return slices.Clone(b.txs)
}

// Evidences returns a copy of evidences.
func (b *Block) Evidences() *[][]Header {
	if b.evidences == nil {
		return nil
	}
	cloned := slices.Clone(*b.evidences)
	return &cloned
}

// Body returns body of a block.
func (b *Block) Body() *Body {
	if b.evidences == nil {
		return &Body{slices.Clone(b.txs), nil}
	}
	clonedEvidences := slices.Clone(*b.evidences)
	return &Body{slices.Clone(b.txs), &clonedEvidences}
}

// EncodeRLP implements rlp.Encoder.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []any{
		b.header,
		b.txs,
		b.evidences,
	})
}

// DecodeRLP implements rlp.Decoder.
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	payload := struct {
		Header    Header
		Txs       tx.Transactions
		Evidences *[][]Header
	}{}

	if err := s.Decode(&payload); err != nil {
		return err
	}

	*b = Block{
		header:    &payload.Header,
		txs:       payload.Txs,
		evidences: payload.Evidences,
	}
	b.cache.size.Store(rlp.ListSize(size))
	return nil
}

// Size returns block size in bytes.
func (b *Block) Size() thor.StorageSize {
	if cached := b.cache.size.Load(); cached != 0 {
		return thor.StorageSize(cached)
	}
	var size thor.StorageSize
	rlp.Encode(&size, b)
	b.cache.size.Store(uint64(size))
	return size
}

func (b *Block) String() string {
	return fmt.Sprintf(`Block(%v)
%v
Transactions: %v, Evidences: %v`, b.Size(), b.header, b.txs, b.evidences)
}
