package tx

import (
	"math/big"
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// gas used by this tx
	GasUsed *big.Int
	// outputs of clauses in tx
	ClauseOutputs []*ClauseOutput
}

type ClauseOutput struct {
	// returned data of this clause
	ReturnedData []byte
	// logs produced by this clause
	Logs []*Log
}
