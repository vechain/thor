package tx

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
)

// body describes details of a tx.
type txBody struct {
	Clauses   Clauses
	GasPrice  *big.Int
	GasLimit  *big.Int
	Nonce     uint64
	DependsOn *cry.Hash `rlp:"nil"`
}

// txPayload payload of a tx.
type txPayload struct {
	Body      txBody
	Signature []byte
}

// Transaction is an immutable tx type.
type Transaction struct {
	payload txPayload

	cache struct {
		hash *cry.Hash
		from *acc.Address
	}
}

// Hash returns hash of tx.
func (t *Transaction) Hash() cry.Hash {
	// TODO
	if h := t.cache.hash; h != nil {
		return *h
	}

	hw := sha3.NewKeccak256()
	rlp.Encode(hw, &t.payload)

	var h cry.Hash
	hw.Sum(h[:0])
	t.cache.hash = &h
	return h
}

// HashForSigning returns hash of tx excludes signature.
func (t *Transaction) HashForSigning() cry.Hash {
	// TODO
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, &t.payload.Body)
	var h cry.Hash
	hw.Sum(h[:0])
	return h
}

// GasPrice returns gas price.
func (t Transaction) GasPrice() *big.Int {
	if t.payload.Body.GasPrice == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(t.payload.Body.GasPrice)
}

// GasLimit returns gas limit.
func (t Transaction) GasLimit() *big.Int {
	if t.payload.Body.GasLimit == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(t.payload.Body.GasLimit)
}

// Signer returns origin of tx.
func (t Transaction) Signer() (*acc.Address, error) {
	// TODO
	return &acc.Address{}, nil
}

func (t *Transaction) resetCache() {
	t.cache.from = nil
	t.cache.hash = nil
}

// WithSignature create a new tx with signature set.
func (t Transaction) WithSignature(sig []byte) *Transaction {
	t.resetCache()
	sigCpy := append([]byte(nil), sig...)
	t.payload.Signature = sigCpy
	return &t
}

// Encode encodes tx into bytes.
func (t Transaction) Encode() []byte {
	data, err := rlp.EncodeToBytes(&t.payload)
	if err != nil {
		panic(err)
	}
	return data
}

// DecodeTransaction decodes bytes into transaction object.
func DecodeTransaction(data []byte) (*Transaction, error) {
	var payload txPayload
	if err := rlp.DecodeBytes(data, &payload); err != nil {
		return nil, err
	}
	return &Transaction{payload: payload}, nil
}

// Transactions a slice of transactions.
type Transactions []Transaction

// RootHash computes merkle root hash of transactions.
func (txs Transactions) RootHash() cry.Hash {
	return cry.Hash(types.DeriveSha(derivableTxs(txs)))
}

// Copy makes a copy of txs slice
func (txs Transactions) Copy() Transactions {
	return append(Transactions(nil), txs...)
}

// implements DerivableList
type derivableTxs []Transaction

func (txs derivableTxs) Len() int {
	return len(txs)
}
func (txs derivableTxs) GetRlp(i int) []byte {
	return txs[i].Encode()
}

//--

// Builder to make it easy to build transaction.
type Builder struct {
	body txBody
}

// Clauses set clauses.
func (b *Builder) Clauses(cs Clauses) *Builder {
	b.body.Clauses = cs.Copy()
	return b
}

// GasPrice set gas price.
func (b *Builder) GasPrice(price *big.Int) *Builder {
	b.body.GasPrice = new(big.Int).Set(price)
	return b
}

// GasLimit set gas limit.
func (b *Builder) GasLimit(limit *big.Int) *Builder {
	b.body.GasLimit = new(big.Int).Set(limit)
	return b
}

// Nonce set nonce.
func (b *Builder) Nonce(nonce uint64) *Builder {
	b.body.Nonce = nonce
	return b
}

// DependsOn set depended tx.
func (b *Builder) DependsOn(txHash *cry.Hash) *Builder {
	if txHash == nil {
		b.body.DependsOn = nil
	} else {
		h := *txHash
		b.body.DependsOn = &h
	}
	return b
}

// Build build tx object.
func (b Builder) Build() *Transaction {
	tx := Transaction{}
	tx.payload.Body = b.body
	return &tx
}
