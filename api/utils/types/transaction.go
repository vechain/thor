package types

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
)

//RawTransaction a raw transaction
type RawTransaction struct {
	ChainTag     byte                `json:"chainTag"`
	Nonce        math.HexOrDecimal64 `json:"nonce"`
	BlockRef     string              `json:"blockRef"`
	Clauses      Clauses             `json:"clauses,string"`
	GasPriceCoef uint8               `json:"gasPriceCoef"`
	Gas          math.HexOrDecimal64 `json:"gas"`
	DependsOn    *thor.Hash          `json:"dependsOn,string"`
	Sig          string              `json:"sig"`
}

//Transaction transaction
type Transaction struct {
	ChainTag     byte                `json:"chainTag"`
	ID           thor.Hash           `json:"id,string"`
	GasPriceCoef uint8               `json:"gasPriceCoef"`
	Gas          math.HexOrDecimal64 `json:"gas"`
	From         thor.Address        `json:"from,string"`
	DependsOn    *thor.Hash          `json:"dependsOn,string"`
	Clauses      Clauses             `json:"clauses"`

	Index       math.HexOrDecimal64 `json:"index"`
	BlockID     thor.Hash           `json:"blockID,string"`
	BlockNumber uint32              `json:"blockNumber"`
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
		Gas:          math.HexOrDecimal64(tx.Gas()),
		Clauses:      cls,
	}
	if tx.DependsOn() != nil {
		t.DependsOn = tx.DependsOn()
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
		builder.Gas(uint64(rawTransaction.Gas))
	}
	blockref, err := hexutil.Decode(rawTransaction.BlockRef)
	if err != nil {
		return nil, fmt.Errorf("invalid blockref %v", err)
	}
	var blkRef tx.BlockRef
	copy(blkRef[:], blockref[:])
	builder.DependsOn(rawTransaction.DependsOn).BlockRef(blkRef).Nonce(uint64(rawTransaction.Nonce))
	for _, clause := range rawTransaction.Clauses {
		v := big.Int(*clause.Value)
		c := tx.NewClause(clause.To).WithValue(&v)
		if clause.Data != "" {
			data, err := hexutil.Decode(clause.Data)
			if err != nil {
				return nil, err
			}
			c.WithData(data)
		}
		builder.Clause(c)
	}
	sig, err := hexutil.Decode(rawTransaction.Sig)
	if err != nil {
		return nil, err
	}
	return builder.Build().WithSignature(sig), nil
}

func (raw *RawTransaction) String() string {
	return fmt.Sprintf(`Clause(
		ChainTag    	 		%v
		Nonce      				%v
		BlockRef					%v
		Clauses      			%v
		GasPriceCoef 			%v
		Gas          			%v
		DependsOn    			%v
		Sig          			%v
		)`, raw.ChainTag,
		raw.Nonce,
		raw.BlockRef,
		raw.Clauses,
		raw.GasPriceCoef,
		raw.Gas,
		raw.DependsOn,
		raw.Sig)
}
