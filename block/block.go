// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/metric"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Block is an immutable block type.
type Block struct {
	header *Header
	txs    tx.Transactions
	bss    ComplexSignatures
	cache  struct {
		size      atomic.Value
		committee atomic.Value
	}
}

// Body defines body of a block.
type Body struct {
	Txs tx.Transactions
	Bss ComplexSignatures
}

type committee struct {
	addrs []thor.Address
	betas [][]byte
}

// Compose compose a block with all needed components
// Note: This method is usually to recover a block by its portions, and the TxsRoot is not verified.
// To build up a block, use a Builder.
func Compose(header *Header, txs tx.Transactions, bss ComplexSignatures) *Block {
	return &Block{
		header: header,
		txs:    append(tx.Transactions(nil), txs...),
		bss:    append(ComplexSignatures(nil), bss...),
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
func (b *Block) BackerSignatures() ComplexSignatures {
	return append(ComplexSignatures(nil), b.bss...)
}

// Body returns body of a block.
func (b *Block) Body() *Body {
	return &Body{
		append(tx.Transactions(nil), b.txs...),
		append(ComplexSignatures(nil), b.bss...),
	}
}

// Committee verifies all backer signatures.
func (b *Block) Committee() ([]thor.Address, [][]byte, error) {
	if cached := b.cache.committee.Load(); cached != nil {
		c := cached.(*committee)
		return c.addrs, c.betas, nil
	}

	if len(b.bss) > 0 {
		var cmt committee
		cmt.addrs = make([]thor.Address, 0, len(b.bss))
		cmt.betas = make([][]byte, 0, len(b.bss))

		hash := NewProposal(b.header.ParentID(), b.header.TxsRoot(), b.header.GasLimit(), b.header.Timestamp()).Hash()
		for _, bs := range b.bss {
			pub, err := crypto.SigToPub(hash.Bytes(), bs.Signature())
			if err != nil {
				return nil, nil, err
			}
			cmt.addrs = append(cmt.addrs, thor.Address(crypto.PubkeyToAddress(*pub)))

			beta, err := ecvrf.NewSecp256k1Sha256Tai().Verify(pub, b.header.Alpha(), bs.Proof())
			if err != nil {
				return nil, nil, err
			}
			cmt.betas = append(cmt.betas, beta)
		}

		b.cache.committee.Store(&cmt)
		return cmt.addrs, cmt.betas, nil
	}

	return nil, nil, nil
}

// EncodeRLP implements rlp.Encoder.
func (b *Block) EncodeRLP(w io.Writer) error {
	input := []interface{}{
		b.header,
		b.txs,
	}

	if len(b.bss) > 0 {
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
