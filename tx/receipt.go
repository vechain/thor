package tx

// Receipt represents the results of a transaction.
type Receipt struct {
	// gas used by this tx
	GasUsed uint64
	// outputs of clauses in tx
	Outputs []*Output
}

// Output output of clause execution.
type Output struct {
	// logs produced by the clause
	Logs []*Log
}
