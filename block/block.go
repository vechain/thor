package block

import (
	"math/big"

	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
	"github.com/vechain/vecore/tx"
)

// payload contains all things in a block.
type payload struct {
	Header    Header
	Signature []byte
	Txs       tx.Transactions
}

// Block is an immutable block type.
type Block struct {
	payload payload

	cache struct {
		hash *cry.Hash
	}
}

func (b *Block) resetCache() {
	b.cache.hash = nil
}

// WithSignature create a new block object with signature set.
func (b Block) WithSignature(sig []byte) *Block {
	b.resetCache()
	b.payload.Signature = append([]byte(nil), sig...)
	return &b
}

// Hash computes hash of block.
func (b *Block) Hash() cry.Hash {
	if h := b.cache.hash; h != nil {
		return *h
	}
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, &b.payload)

	var h cry.Hash
	hw.Sum(h[:0])
	b.cache.hash = &h
	return h
}

// HashForSigning computes hash of block excludes signature.
func (b Block) HashForSigning() cry.Hash {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, &b.payload.Header)

	var h cry.Hash
	hw.Sum(h[:0])
	return h
}

// Header returns a copy of block header.
func (b Block) Header() *Header {
	return b.payload.Header.Copy()
}

// Transactions returns a copy of transactions.
func (b Block) Transactions() tx.Transactions {
	return b.payload.Txs.Copy()
}

// Signer returns signer of block
func (b Block) Signer() (*acc.Address, error) {
	// TODO
	return &acc.Address{}, nil
}

//--

// Builder to make it easy to build a block object.
type Builder struct {
	header Header
	txs    tx.Transactions
}

// ParentHash set parent hash.
func (b *Builder) ParentHash(hash cry.Hash) *Builder {
	b.header.ParentHash = hash
	return b
}

// Timestamp set timestamp.
func (b *Builder) Timestamp(ts uint64) *Builder {
	b.header.Timestamp = ts
	return b
}

// GasLimit set gas limit.
func (b *Builder) GasLimit(limit *big.Int) *Builder {
	b.header.GasLimit = new(big.Int).Set(limit)
	return b
}

// GasUsed set gas used.
func (b *Builder) GasUsed(used *big.Int) *Builder {
	b.header.GasUsed = new(big.Int).Set(used)
	return b
}

// RewardRecipient set recipient of reward.
func (b *Builder) RewardRecipient(addr *acc.Address) *Builder {
	if addr == nil {
		b.header.RewardRecipient = nil
	} else {
		cpy := *addr
		b.header.RewardRecipient = &cpy
	}
	return b
}

// StateRoot set state root.
func (b *Builder) StateRoot(hash cry.Hash) *Builder {
	b.header.StateRoot = hash
	return b
}

// ReceiptsRoot set receipts root.
func (b *Builder) ReceiptsRoot(hash cry.Hash) *Builder {
	b.header.ReceiptsRoot = hash
	return b
}

// Transactions set transactions.
func (b *Builder) Transactions(txs tx.Transactions) *Builder {
	b.header.TxsRoot = txs.RootHash()
	b.txs = txs.Copy()
	return b
}

// Build build a block object.
func (b Builder) Build() *Block {
	return &Block{
		payload: payload{
			Header: b.header,
			Txs:    b.txs,
		},
	}
}
