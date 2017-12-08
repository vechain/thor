package tx

import (
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/dsa"
)

// Transaction is an immutable tx type.
type Transaction struct {
	body      body
	signature []byte

	cache struct {
		hash   *cry.Hash
		signer *acc.Address
	}
}

// body describes details of a tx.
type body struct {
	Clauses   Clauses
	GasPrice  *big.Int
	GasLimit  *big.Int
	Nonce     uint64
	DependsOn *cry.Hash `rlp:"nil"`
}

// Hash returns hash of tx.
func (t *Transaction) Hash() cry.Hash {
	if cached := t.cache.hash; cached != nil {
		return *cached
	}

	hw := cry.NewHasher()
	rlp.Encode(hw, t)

	var h cry.Hash
	hw.Sum(h[:0])
	t.cache.hash = &h
	return h
}

// HashForSigning returns hash of tx excludes signature.
func (t *Transaction) HashForSigning() cry.Hash {
	hw := cry.NewHasher()
	rlp.Encode(hw, &t.body)
	var h cry.Hash
	hw.Sum(h[:0])
	return h
}

// GasPrice returns gas price.
func (t *Transaction) GasPrice() *big.Int {
	if t.body.GasPrice == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(t.body.GasPrice)
}

// GasLimit returns gas limit.
func (t *Transaction) GasLimit() *big.Int {
	if t.body.GasLimit == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(t.body.GasLimit)
}

// WithSignature create a new tx with signature set.
func (t *Transaction) WithSignature(sig []byte) *Transaction {
	sigCpy := append([]byte(nil), sig...)
	return &Transaction{
		body:      t.body,
		signature: sigCpy,
	}
}

// Signer returns the signer of tx.
func (t *Transaction) Signer() (*acc.Address, error) {
	if len(t.signature) == 0 {
		return nil, errors.New("not signed")
	}
	if signer := t.cache.signer; signer != nil {
		cpy := *signer
		return &cpy, nil
	}
	signer, err := dsa.Signer(t.HashForSigning(), t.signature)
	if err != nil {
		return nil, err
	}
	t.cache.signer = signer
	cpy := *signer
	return &cpy, nil
}

// EncodeRLP implements rlp.Encoder
func (t *Transaction) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		t.body,
		t.signature,
	})
}

// Decoder to decode bytes into tx object.
// Since Transaction is immutable, it's not suitable to implement rlp.Decoder directly.
type Decoder struct {
	*Transaction
}

// DecodeRLP implements rlp.Decoder
func (d *Decoder) DecodeRLP(s *rlp.Stream) error {
	payload := struct {
		B body
		S []byte
	}{}
	if err := s.Decode(&payload); err != nil {
		return err
	}
	d.Transaction = &Transaction{
		body:      payload.B,
		signature: payload.S,
	}
	return nil
}
