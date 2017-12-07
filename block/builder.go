package block

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/tx"
)

// Builder to make it easy to build a block object.
type Builder struct {
	subject subject
	txs     tx.Transactions
}

// ParentHash set parent hash.
func (b *Builder) ParentHash(hash cry.Hash) *Builder {
	b.subject.ParentHash = hash
	return b
}

// Timestamp set timestamp.
func (b *Builder) Timestamp(ts uint64) *Builder {
	b.subject.Timestamp = ts
	return b
}

// GasLimit set gas limit.
func (b *Builder) GasLimit(limit *big.Int) *Builder {
	b.subject.GasLimit = new(big.Int).Set(limit)
	return b
}

// GasUsed set gas used.
func (b *Builder) GasUsed(used *big.Int) *Builder {
	b.subject.GasUsed = new(big.Int).Set(used)
	return b
}

// Beneficiary set recipient of reward.
func (b *Builder) Beneficiary(addr acc.Address) *Builder {
	b.subject.Beneficiary = addr
	return b
}

// StateRoot set state root.
func (b *Builder) StateRoot(hash cry.Hash) *Builder {
	b.subject.StateRoot = hash
	return b
}

// ReceiptsRoot set receipts root.
func (b *Builder) ReceiptsRoot(hash cry.Hash) *Builder {
	b.subject.ReceiptsRoot = hash
	return b
}

// Transaction add a transaction.
func (b *Builder) Transaction(tx *tx.Transaction) *Builder {
	b.txs = append(b.txs, tx)
	return b
}

// Build build a block object.
func (b *Builder) Build() *Block {
	header := Header{
		subject: b.subject,
	}
	header.subject.TxsRoot = b.txs.RootHash()

	return &Block{
		header: &header,
		txs:    b.txs.Copy(),
	}
}
