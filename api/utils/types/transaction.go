package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
)

//RawTransaction a raw transaction
type RawTransaction struct {
	ChainTag     byte    `json:"chainTag"`
	Nonce        uint64  `json:"nonce,string"`
	BlockRef     string  `json:"blockRef"`
	Clauses      Clauses `json:"clauses,string"`
	GasPriceCoef uint8   `json:"gasPriceCoef"`
	Gas          uint64  `json:"gas,string"`
	DependsOn    string  `json:"dependsOn"`
	Sig          string  `json:"sig"`
}

//Transaction transaction
type Transaction struct {
	ChainTag     byte    `json:"chainTag"`
	ID           string  `json:"id"`
	GasPriceCoef uint8   `json:"gasPriceCoef"`
	Gas          uint64  `json:"gas,string"`
	From         string  `json:"from,"`
	DependsOn    string  `json:"dependsOn,"`
	Clauses      Clauses `json:"clauses"`

	Index       uint64 `json:"index,string"`
	BlockID     string `json:"blockID"`
	BlockNumber uint32 `json:"blockNumber"`
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
		t.DependsOn = tx.DependsOn().String()
	}
	return t, nil

}

//BuildRawTransaction returns tx.Builder
func BuildRawTransaction(rawTransaction *RawTransaction) (*tx.Transaction, error) {
	builder := new(tx.Builder)
	builder.ChainTag(rawTransaction.ChainTag)
	if rawTransaction.GasPriceCoef != 0 {
		builder.GasPriceCoef(rawTransaction.GasPriceCoef)
	}
	if rawTransaction.Gas > 0 {
		builder.Gas(rawTransaction.Gas)
	}
	var dependsOn *thor.Hash
	if rawTransaction.DependsOn != "" {
		depTxID, err := thor.ParseHash(rawTransaction.DependsOn)
		if err != nil {
			return nil, err
		}
		dependsOn = &depTxID
	}
	blockref, err := hexutil.Decode(rawTransaction.BlockRef)
	if err != nil {
		return nil, err
	}
	var blkRef tx.BlockRef
	copy(blkRef[:], blockref[:])
	builder.DependsOn(dependsOn).BlockRef(blkRef).Nonce(rawTransaction.Nonce)
	for _, clause := range rawTransaction.Clauses {
		v := big.Int(*clause.Value)
		to, err := thor.ParseAddress(clause.To)
		if err != nil {
			return nil, err
		}
		data, err := hexutil.Decode(clause.Data)
		if err != nil {
			return nil, err
		}
		c := tx.NewClause(&to).WithData(data).WithValue(&v)
		builder.Clause(c)
	}
	sig, err := hexutil.Decode(rawTransaction.Sig)
	if err != nil {
		return nil, err
	}
	return builder.Build().WithSignature(sig), nil
}
