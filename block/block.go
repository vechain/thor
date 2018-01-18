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

type body struct {
	Txs tx.Transactions
}

// Body defines body of a block.
type Body struct {
	body body
}

// EncodeRLP implements rlp.Encoder.
func (b *Body) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &b.body)
}

// DecodeRLP implements rlp.Decoder.
func (b *Body) DecodeRLP(s *rlp.Stream) error {
	var body body
	if err := s.Decode(&body); err != nil {
		return err
	}
	*b = Body{body}
	return nil
}

// New create a block instance.
// Note: This method is usually to recover a block by its portions, and the TxsRoot is not verified.
// To build up a block, use a Builder.
func New(header *Header, txs tx.Transactions) *Block {
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

// ParentHash same as Header.ParentHash().
func (b *Block) ParentHash() thor.Hash {
	return b.header.ParentHash()
}

// Hash same as Header.Hash().
func (b *Block) Hash() thor.Hash {
	return b.header.Hash()
}

// Header returns the block header.
func (b *Block) Header() *Header {
	return b.header
}

// NewTransactionIterator returns a transaction iterator.
func (b *Block) NewTransactionIterator() TransactionIterator {
	return &txIter{txs: b.txs}
}

// GetTransactionCount returns count of transactions contained in this block.
func (b *Block) GetTransactionCount() int {
	return len(b.txs)
}

// Body returns body of a block.
func (b *Block) Body() *Body {
	return &Body{body{b.txs}}
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

// TransactionIterator to iterates txs contained in the block.
type TransactionIterator interface {
	HasNext() bool
	Next() *tx.Transaction
}

type txIter struct {
	txs tx.Transactions
	i   int
}

func (ti *txIter) HasNext() bool {
	return ti.i < len(ti.txs)
}

func (ti *txIter) Next() (tx *tx.Transaction) {
	tx = ti.txs[ti.i]
	ti.i++
	return
}
