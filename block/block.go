// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
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
	header    *Header
	txs       tx.Transactions
	committee *Committee
	cache     struct {
		size atomic.Value
	}
}

// Compose compose a block with all needed components
// Note: This method is usually to recover a block by its portions, and the TxsRoot is not verified.
// To build up a block, use a Builder.
func Compose(header *Header, txs tx.Transactions, cmt *Committee) *Block {
	return &Block{
		header:    header,
		txs:       append(tx.Transactions(nil), txs...),
		committee: cmt,
	}
}

// WithSignature create a new block object with signature set.
func (b *Block) WithSignature(sig []byte) *Block {
	return &Block{
		header:    b.header.withSignature(sig),
		txs:       b.txs,
		committee: b.committee,
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

// Committee returns the committee in block body.
func (b *Block) Committee() *Committee {
	return b.committee
}

// EncodeRLP implements rlp.Encoder.
func (b *Block) EncodeRLP(w io.Writer) error {
	input := []interface{}{
		b.header,
		b.txs,
	}

	if len(b.committee.bss) > 0 {
		input = append(input, b.committee.bss)
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
		bss    ComplexSignatures
	)

	if err := s.Decode(&raws); err != nil {
		return err
	}

	if len(raws) == 3 {
		if err := rlp.DecodeBytes(raws[2], &bss); err != nil {
			return err
		}
		if len(bss) == 0 {
			return errors.New("rlp: block body should be trimmed")
		}
	} else if len(raws) != 2 {
		return errors.New("rlp:invalid fields of block body, want 2 or 3")
	}

	if err := rlp.DecodeBytes(raws[0], &header); err != nil {
		return err
	}

	if err := rlp.DecodeBytes(raws[1], &txs); err != nil {
		return err
	}

	proposalHash := NewProposal(header.ParentID(), header.TxsRoot(), header.GasLimit(), header.Timestamp()).Hash()
	cmt := NewCommittee(proposalHash, header.Alpha(), bss)
	*b = Block{
		header:    &header,
		txs:       txs,
		committee: cmt,
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
