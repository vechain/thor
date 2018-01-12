package tx

import (
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/params"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/bn"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/thor"
)

// Transaction is an immutable tx type.
type Transaction struct {
	body body

	cache struct {
		hash *thor.Hash
	}
}

var _ cry.Signable = (*Transaction)(nil)

// body describes details of a tx.
type body struct {
	Clauses     Clauses
	GasPrice    bn.Int
	Gas         uint64
	Nonce       uint64
	TimeBarrier uint64
	DependsOn   *thor.Hash `rlp:"nil"`
	Signature   []byte
}

// Hash returns hash of tx.
func (t *Transaction) Hash() thor.Hash {
	if cached := t.cache.hash; cached != nil {
		return *cached
	}

	hw := cry.NewHasher()
	rlp.Encode(hw, t)

	var h thor.Hash
	hw.Sum(h[:0])
	t.cache.hash = &h
	return h
}

// HashOfWorkProof returns hash for work proof.
func (t *Transaction) HashOfWorkProof() (hash thor.Hash) {
	hw := cry.NewHasher()
	rlp.Encode(hw, []interface{}{
		t.body.Clauses,
		t.body.GasPrice,
		t.body.Gas,
		t.body.Nonce,
		t.body.TimeBarrier,
		t.body.DependsOn,
	})
	hw.Sum(hash[:0])
	return
}

// SigningHash returns hash of tx excludes signature.
func (t *Transaction) SigningHash() thor.Hash {
	wph := t.HashOfWorkProof()
	// use hash of work proof hash as signing hash
	return cry.HashSum(wph[:])
}

// GasPrice returns gas price.
func (t *Transaction) GasPrice() bn.Int {
	return t.body.GasPrice
}

// Gas returns gas provision for this tx.
func (t *Transaction) Gas() uint64 {
	return t.body.Gas
}

// TimeBarrier returns time barrier.
// It's required that tx.TimeBarrier <= block.Timestamp,
// when a tx was packed in a block.
func (t *Transaction) TimeBarrier() uint64 {
	return t.body.TimeBarrier
}

// Clauses returns caluses in tx.
func (t *Transaction) Clauses() Clauses {
	clauses := make(Clauses, len(t.body.Clauses))
	for i, c := range t.body.Clauses {
		clauses[i] = c.Copy()
	}
	return clauses
}

// Signature returns signature.
func (t *Transaction) Signature() []byte {
	return append([]byte(nil), t.body.Signature...)
}

// WithSignature create a new tx with signature set.
func (t *Transaction) WithSignature(sig []byte) *Transaction {
	newTx := Transaction{
		body: t.body,
	}
	// copy sig
	newTx.body.Signature = append([]byte(nil), sig...)
	return &newTx
}

// EncodeRLP implements rlp.Encoder
func (t *Transaction) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &t.body)
}

// DecodeRLP implements rlp.Decoder
func (t *Transaction) DecodeRLP(s *rlp.Stream) error {
	var body body
	if err := s.Decode(&body); err != nil {
		return err
	}
	*t = Transaction{
		body: body,
	}
	return nil
}

// IntrinsicGas returns intrinsic gas of tx.
// That's sum of all clauses intrinsic gas.
func (t *Transaction) IntrinsicGas() (uint64, error) {
	clauseCount := len(t.body.Clauses)
	if clauseCount == 0 {
		return params.TxGas, nil
	}

	firstClause := t.body.Clauses[0]
	total := core.IntrinsicGas(firstClause.Data, firstClause.To == nil, true)

	for _, c := range t.body.Clauses[1:] {
		contractCreation := c.To == nil
		total.Add(total, core.IntrinsicGas(c.Data, contractCreation, true))

		// sub over-payed gas for clauses after the first one.
		if contractCreation {
			total.Sub(total, new(big.Int).SetUint64(params.TxGasContractCreation-thor.ClauseGasContractCreation))
		} else {
			total.Sub(total, new(big.Int).SetUint64(params.TxGas-thor.ClauseGas))
		}
	}

	if total.BitLen() > 64 {
		return 0, errors.New("intrinsic gas too large")
	}
	return total.Uint64(), nil
}
