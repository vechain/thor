// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
	ComplexSigSize = 81 + 65
)

// MaxTxsPerBlock bounds the transactions decoded from one block body. A valid
// block cannot hold more than gasLimit/21000 txs, so 8192 covers a 172M gas
// limit — 4.3x the current 40M.
const MaxTxsPerBlock = 8192

// Block is an immutable block type.
type Block struct {
	header *Header
	txs    tx.Transactions
	cache  struct {
		size atomic.Uint64
	}
}

// Body defines body of a block.
type Body struct {
	Txs tx.Transactions
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

// Body returns body of a block.
func (b *Block) Body() *Body {
	return &Body{slices.Clone(b.txs)}
}

// EncodeRLP implements rlp.Encoder.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []any{
		b.header,
		b.txs,
	})
}

// DecodeRLP implements rlp.Decoder.
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()

	if _, err := s.List(); err != nil {
		return err
	}
	var header Header
	if err := s.Decode(&header); err != nil {
		if err == rlp.EOL {
			// Returning the EOL sentinel would let an enclosing list decoder read it
			// as "end of list" and silently drop the block.
			return errors.New("rlp: too few elements for block")
		}
		return err
	}
	txs, err := decodeTxs(s)
	if err != nil {
		if err == rlp.EOL {
			// Same reasoning: EOL here means the tx list element is absent.
			return errors.New("rlp: too few elements for block")
		}
		return err
	}
	// The outer list must hold exactly two items. Mapping every error is total:
	// after a successful s.List only errNotAtEOL is reachable, and rlp's own
	// wording for it says nothing about blocks.
	if err := s.ListEnd(); err != nil {
		return errors.New("rlp: too many elements for block")
	}

	*b = Block{header: &header, txs: txs}
	b.cache.size.Store(rlp.ListSize(size))
	return nil
}

// decodeTxs decodes a transaction list, stopping as soon as MaxTxsPerBlock is
// exceeded so that a malformed list costs no more than the limit allows.
func decodeTxs(s *rlp.Stream) (tx.Transactions, error) {
	if _, err := s.List(); err != nil {
		return nil, err
	}
	var txs tx.Transactions
	for {
		var t tx.Transaction
		err := s.Decode(&t)
		if err == rlp.EOL {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(txs) == MaxTxsPerBlock {
			return nil, fmt.Errorf("tx count exceeds limit: > %d", MaxTxsPerBlock)
		}
		txs = append(txs, &t)
	}
	if err := s.ListEnd(); err != nil {
		return nil, err
	}
	return txs, nil
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
Transactions: %v`, b.Size(), b.header, b.txs)
}
