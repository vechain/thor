package tx

import (
	"math/big"

	"github.com/vechain/vecore/acc"
)

// Clause is the basic execution unit of a transaction.
type Clause struct {
	To    *acc.Address `rlp:"nil"`
	Value *big.Int
	Data  []byte
}

// Copy makes a deep copy of clause.
func (c Clause) Copy() *Clause {
	if c.To != nil {
		to := *c.To
		c.To = &to
	}

	value := new(big.Int)
	if c.Value != nil {
		value.Set(c.Value)
	}
	c.Value = value

	c.Data = append([]byte(nil), c.Data...)
	return &c
}

// Clauses array of clauses.
type Clauses []*Clause

// Copy returns a shallow copy.
func (cs Clauses) Copy() Clauses {
	return append(Clauses(nil), cs...)
}
