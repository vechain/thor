package types

import (
	"github.com/vechain/thor/tx"
	"math/big"
)

//Transaction transaction
type Transaction struct {
	ID          string
	Index       uint64
	BlockID     string
	BlockNumber uint32
	GasPrice    *big.Int
	Gas         uint64
	From        string

	Clauses Clauses
}

//Transactions transactions
type Transactions []*Transaction

//ConvertTransaction convert a raw transaction into a json format transaction
func ConvertTransaction(tx *tx.Transaction) (*Transaction, error) {
	//tx signer
	from, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	//copy tx hash
	cls := make(Clauses, len(tx.Clauses()))
	for i, c := range tx.Clauses() {
		cls[i] = ConvertClause(c)
	}

	return &Transaction{
		ID:       tx.ID().String(),
		From:     from.String(),
		GasPrice: tx.GasPrice(),
		Gas:      tx.Gas(),
		Clauses:  cls,
	}, nil

}
