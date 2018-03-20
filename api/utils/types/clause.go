package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/tx"
)

// Clause for json marshal
type Clause struct {
	To    string                `json:"to"`
	Value *math.HexOrDecimal256 `json:"value,string"`
	Data  string                `json:"data"`
}

//Clauses array of clauses.
type Clauses []Clause

//ConvertClause convert a raw clause into a json format clause
func ConvertClause(c *tx.Clause) Clause {
	v := math.HexOrDecimal256(*c.Value())
	return Clause{
		c.To().String(),
		&v,
		hexutil.Encode(c.Data()),
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
