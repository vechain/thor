package types

import (
	"github.com/vechain/thor/tx"
	"math/big"
)

// Clause for json marshal
type Clause struct {
	To    string `rlp:"nil"`
	Value *big.Int
	Data  []byte
}

//Clauses array of clauses.
type Clauses []Clause

//ConvertClause convert a raw clause into a jason format clause
func ConvertClause(c *tx.Clause) Clause {

	return Clause{
		c.To.String(),
		c.Value,
		c.Data,
	}

}

//Do iterate clauses
func (cs Clauses) Do(fn func(c Clause) bool) {
	for _, v := range cs {
		if !fn(v) {
			break
		}
	}
}
