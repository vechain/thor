package block

import (
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
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

// Compose compose a block with all needed components
// Note: This method is usually to recover a block by its portions, and the TxsRoot is not verified.
// To build up a block, use a Builder.
func Compose(header *Header, txs tx.Transactions) *Block {
	return &Block{
		header,
		append(tx.Transactions(nil), txs...),
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
func (b *Block) TotalScore() uint64 {
	return b.header.TotalScore()
}

// ParentID same as Header.ParentID().
func (b *Block) ParentID() thor.Hash {
	return b.header.ParentID()
}

// ID same as Header.ID().
func (b *Block) ID() thor.Hash {
	return b.header.ID()
}

// Signer same as Header.Signer
func (b *Block) Signer() (thor.Address, error) {
	return b.header.Signer()
}

// Header returns the block header.
func (b *Block) Header() *Header {
	return b.header
}

// Transactions returns a copy of transactions.
func (b *Block) Transactions() tx.Transactions {
	return append(tx.Transactions(nil), b.txs...)
}

// Body returns body of a block.
func (b *Block) Body() *Body {
	return &Body{append(tx.Transactions(nil), b.txs...)}
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
