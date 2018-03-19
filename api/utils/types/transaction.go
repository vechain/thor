package types

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
)

//RawTransaction a raw transaction
type RawTransaction struct {
	ChainTag     byte       `json:"chainTag"`
	Nonce        uint64     `json:"nonce,string"`
	BlockRef     [8]byte    `json:"blockRef,[8]byte"`
	Clauses      Clauses    `json:"clauses,string"`
	GasPriceCoef uint8      `json:"gasPriceCoef"`
	Gas          uint64     `json:"gas,string"`
	DependsOn    *thor.Hash `json:"dependsOn,string"`
	Sig          []byte     `json:"sig,[]byte"`
}

//Transaction transaction
type Transaction struct {
	ChainTag     byte         `json:"chainTag"`
	ID           thor.Hash    `json:"id,string"`
	GasPriceCoef uint8        `json:"gasPriceCoef"`
	Gas          uint64       `json:"gas,string"`
	From         thor.Address `json:"from,string"`
	DependsOn    *thor.Hash   `json:"dependsOn,string"`
	Clauses      Clauses      `json:"clauses,string"`

	Index       uint64    `json:"index,string"`
	BlockID     thor.Hash `json:"blockID,string"`
	BlockNumber uint32    `json:"blockNumber"`
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
		ID:           tx.ID(),
		From:         from,
		GasPriceCoef: tx.GasPriceCoef(),
		Gas:          tx.Gas(),
		Clauses:      cls,
	}
	if tx.DependsOn() != nil {
		t.DependsOn = tx.DependsOn()
	}
	return t, nil

}

//BuildRawTransaction returns tx.Builder
func BuildRawTransaction(rawTransaction *RawTransaction) (*tx.Builder, error) {
	builder := new(tx.Builder)
	builder.ChainTag(rawTransaction.ChainTag)
	if rawTransaction.GasPriceCoef != 0 {
		builder.GasPriceCoef(rawTransaction.GasPriceCoef)
	}
	if rawTransaction.Gas > 0 {
		builder.Gas(rawTransaction.Gas)
	}
	builder.DependsOn(rawTransaction.DependsOn).BlockRef(rawTransaction.BlockRef).Nonce(rawTransaction.Nonce)
	for _, clause := range rawTransaction.Clauses {
		v := big.Int(*clause.Value)
		c := tx.NewClause(clause.To).WithData(clause.Data).WithValue(&v)
		builder.Clause(c)
	}
	return builder, nil
}
