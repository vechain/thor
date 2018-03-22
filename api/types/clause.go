package types

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Clause for json marshal
type Clause struct {
	To    *thor.Address         `json:"to,string"`
	Value *math.HexOrDecimal256 `json:"value,string"`
	Data  string                `json:"data"`
}

//Clauses array of clauses.
type Clauses []Clause

//ConvertClause convert a raw clause into a json format clause
func ConvertClause(c *tx.Clause) Clause {
	v := math.HexOrDecimal256(*c.Value())
	return Clause{
		c.To(),
		&v,
		hexutil.Encode(c.Data()),
	}
}

func (c *Clause) String() string {
	return fmt.Sprintf(`Clause(
		To    %v
		Value %v
		Data  %v
		)`, c.To,
		c.Value,
		c.Data)
}
