package tx

import (
	"encoding/binary"

	"github.com/vechain/thor/thor"
)

// Builder to make it easy to build transaction.
type Builder struct {
	body body
}

// ChainTag set chain tag.
func (b *Builder) ChainTag(tag byte) *Builder {
	b.body.ChainTag = tag
	return b
}

// Clause add a clause.
func (b *Builder) Clause(c *Clause) *Builder {
	b.body.Clauses = append(b.body.Clauses, c)
	return b
}

// GasPriceCoef set gas price coef.
func (b *Builder) GasPriceCoef(coef uint8) *Builder {
	b.body.GasPriceCoef = coef
	return b
}

// Gas set gas provision for tx.
func (b *Builder) Gas(gas uint64) *Builder {
	b.body.Gas = gas
	return b
}

// BlockRef set block reference.
func (b *Builder) BlockRef(br BlockRef) *Builder {
	b.body.BlockRef = binary.BigEndian.Uint64(br[:])
	return b
}

// Nonce set nonce.
func (b *Builder) Nonce(nonce uint64) *Builder {
	b.body.Nonce = nonce
	return b
}

// DependsOn set depended tx.
func (b *Builder) DependsOn(txHash *thor.Hash) *Builder {
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
	return &tx
}
