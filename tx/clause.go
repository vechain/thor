package tx

import (
	"math/big"

	"github.com/vechain/thor/thor"
)

// Clause is the basic execution unit of a transaction.
type Clause struct {
	To    *thor.Address `rlp:"nil"`
	Value *big.Int
	Data  []byte
}

// Copy makes a copy of clause.
func (c Clause) Copy() *Clause {
	if c.To != nil {
		to := *c.To
		c.To = &to
	}
	if c.Value != nil {
		c.Value = new(big.Int).Set(c.Value)
	}
	c.Data = append([]byte(nil), c.Data...)
	return &c
}

// Clauses array of clauses.
type Clauses []*Clause

// Copy returns a shallow copy.
func (cs Clauses) Copy() Clauses {
	return append(Clauses(nil), cs...)
}
