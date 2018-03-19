package types

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//RawTransaction a raw transaction
type RawTransaction struct {
	ChainTag     byte
	Nonce        uint64
	BlockRef     [8]byte
	Clauses      Clauses
	GasPriceCoef uint8
	Gas          uint64
	DependsOn    *thor.Hash
	Sig          []byte
}

//Transaction transaction
type Transaction struct {
	ChainTag     byte
	ID           string
	GasPriceCoef uint8
	Gas          uint64
	From         string
	DependsOn    string
	Clauses      Clauses

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
		ChainTag:     tx.ChainTag(),
		ID:           tx.ID().String(),
		From:         from.String(),
		GasPriceCoef: tx.GasPriceCoef(),
		Gas:          tx.Gas(),
		Clauses:      cls,
	}
	if tx.DependsOn() != nil {
		t.DependsOn = (*tx.DependsOn()).String()
	}
	return t, nil

}

//BuildRawTransaction returns tx.Builder
func BuildRawTransaction(rawTransaction *RawTransaction) (*tx.Builder, error) {
	builder := new(tx.Builder)
	if rawTransaction.GasPriceCoef != 0 {
		builder.GasPriceCoef(rawTransaction.GasPriceCoef)
	}
	if rawTransaction.Gas > 0 {
		builder.Gas(rawTransaction.Gas)
	}
	builder.DependsOn(rawTransaction.DependsOn).BlockRef(rawTransaction.BlockRef).Nonce(rawTransaction.Nonce)
	for _, clause := range rawTransaction.Clauses {
		c := tx.NewClause(clause.To).WithData(clause.Data).WithValue(clause.Value)
		builder.Clause(c)
	}
	return builder, nil
}
