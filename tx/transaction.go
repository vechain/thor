package tx

import (
	"encoding/binary"
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/thor"
)

const signerCacheSize = 1024

var signerCache = cache.NewLRU(signerCacheSize)

// Transaction is an immutable tx type.
type Transaction struct {
	body body

	cache struct {
		hash *thor.Hash
		id   *thor.Hash
	}
}

// body describes details of a tx.
type body struct {
	ChainTag  uint32
	Clauses   []*Clause
	GasPrice  *big.Int
	Gas       uint64
	Nonce     uint64
	BlockRef  uint64
	DependsOn *thor.Hash `rlp:"nil"`
	Signature []byte
}

// ChainTag returns chain tag.
func (t *Transaction) ChainTag() uint32 {
	return t.body.ChainTag
}

func (t *Transaction) hash() (hash thor.Hash) {
	if cached := t.cache.hash; cached != nil {
		return *cached
	}

	hw := sha3.NewKeccak256()
	rlp.Encode(hw, t)

	hw.Sum(hash[:0])
	t.cache.hash = &hash
	return hash
}

// ID returns id of tx.
func (t *Transaction) ID() (id thor.Hash) {
	if cached := t.cache.id; cached != nil {
		return *cached
	}
	signer, err := t.Signer()
	if err != nil {
		return thor.Hash{}
	}
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, []interface{}{
		t.body.ChainTag,
		t.body.Clauses,
		t.body.GasPrice,
		t.body.Gas,
		t.body.Nonce,
		t.body.BlockRef,
		t.body.DependsOn,
		signer,
	})
	hw.Sum(id[:0])
	t.cache.id = &id
	return
}

// SigningHash returns hash of tx excludes signature.
func (t *Transaction) SigningHash() (hash thor.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, []interface{}{
		t.body.ChainTag,
		t.body.Clauses,
		t.body.GasPrice,
		t.body.Gas,
		t.body.Nonce,
		t.body.BlockRef,
		t.body.DependsOn,
	})
	hw.Sum(hash[:0])
	return
}

// GasPrice returns gas price.
func (t *Transaction) GasPrice() *big.Int {
	return new(big.Int).Set(t.body.GasPrice)
}

// Gas returns gas provision for this tx.
func (t *Transaction) Gas() uint64 {
	return t.body.Gas
}

// BlockRef returns block reference, which is first 8 bytes of block hash.
func (t *Transaction) BlockRef() (br BlockRef) {
	binary.BigEndian.PutUint64(br[:], t.body.BlockRef)
	return
}

// Clauses returns caluses in tx.
func (t *Transaction) Clauses() []*Clause {
	return append([]*Clause(nil), t.body.Clauses...)
}

// DependsOn returns depended tx hash.
func (t *Transaction) DependsOn() *thor.Hash {
	if t.body.DependsOn == nil {
		return nil
	}
	cpy := *t.body.DependsOn
	return &cpy
}

// Signature returns signature.
func (t *Transaction) Signature() []byte {
	return append([]byte(nil), t.body.Signature...)
}

// Signer extract signer of tx from signature.
func (t *Transaction) Signer() (thor.Address, error) {
	cacheKey := t.hash()
	if v, ok := signerCache.Get(cacheKey); ok {
		return v.(thor.Address), nil
	}
	pub, err := crypto.SigToPub(t.SigningHash().Bytes(), t.body.Signature)
	if err != nil {
		return thor.Address{}, err
	}
	signer := thor.Address(crypto.PubkeyToAddress(*pub))
	signerCache.Add(cacheKey, signer)
	return signer, nil
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
	total := core.IntrinsicGas(firstClause.body.Data, firstClause.body.To == nil, true)

	for _, c := range t.body.Clauses[1:] {
		contractCreation := c.body.To == nil
		total.Add(total, core.IntrinsicGas(c.body.Data, contractCreation, true))

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
