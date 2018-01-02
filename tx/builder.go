package tx

import (
	"math/big"

	"github.com/vechain/thor/cry"
)

// Builder to make it easy to build transaction.
type Builder struct {
	body body
}

// Clause add a clause.
func (b *Builder) Clause(c *Clause) *Builder {
	b.body.Clauses = append(b.body.Clauses, c.Copy())
	return b
}

// GasPrice set gas price.
func (b *Builder) GasPrice(price *big.Int) *Builder {
	b.body.GasPrice.SetBig(price)
	return b
}

// GasLimit set gas limit.
func (b *Builder) GasLimit(limit *big.Int) *Builder {
	b.body.GasLimit.SetBig(limit)
	return b
}

// TimeBarrier set time barrier.
func (b *Builder) TimeBarrier(tb uint64) *Builder {
	b.body.TimeBarrier = tb
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
		cpy := *txHash
		b.body.DependsOn = &cpy
	}
	return b
}

// Build build tx object.
func (b *Builder) Build() *Transaction {
	tx := Transaction{body: b.body}
	tx.body.Clauses = b.body.Clauses.Copy()
	return &tx
}
