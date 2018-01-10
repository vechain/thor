package tx

import (
	"github.com/vechain/thor/bn"
	"github.com/vechain/thor/thor"
)

// Clause is the basic execution unit of a transaction.
type Clause struct {
	To    *thor.Address `rlp:"nil"`
	Value bn.Int
	Data  []byte
}

// Copy makes a copy of clause.
func (c Clause) Copy() *Clause {
	if c.To != nil {
		to := *c.To
		c.To = &to
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
