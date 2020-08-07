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
	header *Header
	txs    tx.Transactions
	bss    BackerSignatures
	cache  struct {
		size atomic.Value
	}
}

// Body defines body of a block.
type Body struct {
	Txs tx.Transactions
	Bss BackerSignatures
}

// Compose compose a block with all needed components
// Note: This method is usually to recover a block by its portions, and the TxsRoot is not verified.
// To build up a block, use a Builder.
func Compose(header *Header, txs tx.Transactions, bss BackerSignatures) *Block {
	return &Block{
		header: header,
		txs:    append(tx.Transactions(nil), txs...),
		bss:    append(BackerSignatures(nil), bss...),
	}
}

// WithSignature create a new block object with signature set.
func (b *Block) WithSignature(sig []byte) *Block {
	return &Block{
		header: b.header.withSignature(sig),
		txs:    b.txs,
		bss:    b.bss,
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
	return append(BackerSignatures(nil), b.bss...)
}

// Body returns body of a block.
func (b *Block) Body() *Body {
	return &Body{
		append(tx.Transactions(nil), b.txs...),
		append(BackerSignatures(nil), b.bss...),
	}
}

// EncodeRLP implements rlp.Encoder.
func (b *Block) EncodeRLP(w io.Writer) error {
	input := []interface{}{
		b.header,
		b.txs,
	}

	// TotalBackersCount not equal 0 means block is surely at post 193 stage.
	if b.Header().TotalBackersCount() != 0 {
		input = append(input, b.bss)
	}

	return rlp.Encode(w, input)
}

// DecodeRLP implements rlp.Decoder.
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()

	var (
		raws   []rlp.RawValue
		header Header
		txs    tx.Transactions
		bss    BackerSignatures
	)

	if err := s.Decode(&raws); err != nil {
		return err
	}
	if len(raws) < 2 {
		return errors.New("rlp:invalid fields of block body, at least 2")
	}
	if err := rlp.Decode(bytes.NewReader(raws[0]), &header); err != nil {
		return err
	}
	if err := rlp.Decode(bytes.NewReader(raws[1]), &txs); err != nil {
		return err
	}

	// strictly limit the fields of block body in pre and post 193 fork stage.
	// before 193: block must contain only block header and transactions.
	if header.TotalBackersCount() != 0 && len(raws) == 3 {
		if err := rlp.Decode(bytes.NewReader(raws[2]), &bss); err != nil {
			return err
		}
	} else if header.TotalBackersCount() == 0 && len(raws) == 2 {
		bss = BackerSignatures(nil)
	} else {
		return errors.New("rlp:block has too many fields")
	}

	*b = Block{
		header: &header,
		txs:    txs,
		bss:    bss,
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
