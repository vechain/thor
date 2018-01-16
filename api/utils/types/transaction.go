package types

import (
	"github.com/vechain/thor/cry"
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

//Transaction transaction
type Transaction struct {
	Hash        string
	GasPrice    *big.Int
	Gas         uint64
	TimeBarrier uint64
	From        string

	Clauses Clauses
}

//Transactions transactions
type Transactions []*Transaction

//ConvertTransactionWithSigning convert a raw transaction into a json format transaction
func ConvertTransactionWithSigning(tx *tx.Transaction, signing *cry.Signing) *Transaction {
	//tx signer
	from, err := signing.Signer(tx)
	if err != nil {
		return nil
	}
	//copy tx hash
	cls := make(Clauses, len(tx.Clauses()))
	for i, c := range tx.Clauses() {
		cls[i] = ConvertClause(c)
	}

	return &Transaction{
		Hash:        tx.SigningHash().String(),
		From:        from.String(),
		GasPrice:    tx.GasPrice(),
		Gas:         tx.Gas(),
		TimeBarrier: tx.TimeBarrier(),
		Clauses:     cls,
	}

}

//ConvertClause convert a raw clause into a jason format clause
func ConvertClause(c *tx.Clause) Clause {

	return Clause{
		c.To.String(),
		c.Value,
		c.Data,
	}

}
