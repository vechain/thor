package block

import (
	"io"
	"math/big"

	"github.com/vechain/thor/cry"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/tx"
)

// Block is an immutable block type.
type Block struct {
	header *Header
	txs    tx.Transactions
}

// Body defines body of a block.
type Body struct {
	Txs tx.Transactions
}

// New create a block instance.
// Note: This method is usually to recover a block by its portions, and the TxsRoot is not verified.
// To build up a block, use a Builder.
func New(header *Header, txs tx.Transactions) *Block {
	return &Block{
		header,
		txs.Copy(),
	}
}

// WithSignature create a new block object with signature set.
func (b *Block) WithSignature(sig []byte) *Block {
	return &Block{
		b.header.WithSignature(sig),
		b.txs,
	}
}

// Number same as Header.Number().
func (b *Block) Number() uint32 {
	return b.header.Number()
}

// Timestamp same as Header.Timestamp().
func (b *Block) Timestamp() uint64 {
	return b.header.Timestamp()
}

// TotalScore same as Header.TotalScore().
func (b *Block) TotalScore() *big.Int {
	return b.header.TotalScore()
}

// ParentHash same as Header.ParentHash().
func (b *Block) ParentHash() cry.Hash {
	return b.header.ParentHash()
}

// Hash same as Header.Hash().
func (b *Block) Hash() cry.Hash {
	return b.header.Hash()
}

// Header returns a copy of block header.
func (b *Block) Header() *Header {
	return b.header
}

// Transactions returns a copy of transactions.
func (b *Block) Transactions() tx.Transactions {
	return b.txs.Copy()
}

// Body returns body of a block.
func (b *Block) Body() *Body {
	return &Body{b.txs.Copy()}
}

// EncodeRLP implements rlp.Encoder.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		b.header,
		b.txs,
	})
}

// DecodeRLP implements rlp.Decoder.
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	payload := struct {
		Header Header
		Txs    tx.Transactions
	}{}

	if err := s.Decode(&payload); err != nil {
		return err
	}

	*b = Block{
		&payload.Header,
		payload.Txs,
	}
	return nil
}
