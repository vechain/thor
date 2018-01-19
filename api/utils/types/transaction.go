package types

import (
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/tx"
	"math/big"
)

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
		Hash:        tx.Hash().String(),
		From:        from.String(),
		GasPrice:    tx.GasPrice(),
		Gas:         tx.Gas(),
		TimeBarrier: tx.TimeBarrier(),
		Clauses:     cls,
	}

}
