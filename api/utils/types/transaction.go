package types

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
)

//RawTransaction a raw transaction
type RawTransaction struct {
	Nonce     uint64
	GasPrice  *big.Int
	Gas       uint64
	DependsOn string
	Sig       []byte
	BlockRef  [8]byte
	Clauses   Clauses
}

//Transaction transaction
type Transaction struct {
	ChainTag  byte
	ID        string
	GasPrice  *big.Int
	Gas       uint64
	From      string
	DependsOn string
	Clauses   Clauses

	Index       uint64
	BlockID     string
	BlockNumber uint32
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
	t := &Transaction{
		ChainTag: tx.ChainTag(),
		ID:       tx.ID().String(),
		From:     from.String(),
		GasPrice: tx.GasPrice(),
		Gas:      tx.Gas(),
		Clauses:  cls,
	}
	if tx.DependsOn() != nil {
		t.DependsOn = (*tx.DependsOn()).String()
	}
	return t, nil

}

//BuildRawTransaction returns tx.Builder
func BuildRawTransaction(rawTransaction *RawTransaction) (*tx.Builder, error) {
	builder := new(tx.Builder)
	if rawTransaction.GasPrice != nil {
		builder.GasPrice(rawTransaction.GasPrice)
	}
	if rawTransaction.Gas > 0 {
		builder.Gas(rawTransaction.Gas)
	}
	dependsOn, err := thor.ParseHash(rawTransaction.DependsOn)
	if err != nil {
		builder.DependsOn(nil)
	}
	builder.BlockRef(rawTransaction.BlockRef)
	builder.Nonce(rawTransaction.Nonce)
	builder.DependsOn(&dependsOn)
	for _, clause := range rawTransaction.Clauses {
		// to, err := thor.ParseAddress(clause.To)
		// if err != nil {
		// 	return nil, err
		// }
		c := tx.NewClause(clause.To).WithData(clause.Data).WithValue(clause.Value)
		builder.Clause(c)
	}
	return builder, nil
}
