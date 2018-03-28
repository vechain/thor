package transactions

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
)

// Clause for json marshal
type Clause struct {
	To    *thor.Address         `json:"to,string"`
	Value *math.HexOrDecimal256 `json:"value,string"`
	Data  string                `json:"data"`
}

//Clauses array of clauses.
type Clauses []Clause

//ConvertClause convert a raw clause into a json format clause
func ConvertClause(c *tx.Clause) Clause {
	v := math.HexOrDecimal256(*c.Value())
	return Clause{
		c.To(),
		&v,
		hexutil.Encode(c.Data()),
	}
}

func (c *Clause) String() string {
	return fmt.Sprintf(`Clause(
		To    %v
		Value %v
		Data  %v
		)`, c.To,
		c.Value,
		c.Data)
}

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
	BlockID     thor.Hash           `json:"blockID,string"`
	BlockNumber uint32              `json:"blockNumber"`
	TxIndex     math.HexOrDecimal64 `json:"txIndex"`

	ChainTag     byte                `json:"chainTag"`
	ID           thor.Hash           `json:"id,string"`
	GasPriceCoef uint8               `json:"gasPriceCoef"`
	Gas          math.HexOrDecimal64 `json:"gas"`
	From         thor.Address        `json:"from,string"`
	DependsOn    *thor.Hash          `json:"dependsOn,string"`
	Clauses      Clauses             `json:"clauses"`
}

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

func buildRawTransaction(rawTransaction *RawTransaction) (*tx.Transaction, error) {
	builder := new(tx.Builder)
	for _, clause := range rawTransaction.Clauses {
		v := big.Int(*clause.Value)
		c := tx.NewClause(clause.To).WithValue(&v)
		if clause.Data != "" {
			data, err := hexutil.Decode(clause.Data)
			if err != nil {
				return nil, errors.Wrap(err, "data")
			}
			c.WithData(data)
		}
		builder.Clause(c)
	}
	sig, err := hexutil.Decode(rawTransaction.Sig)
	if err != nil {
		return nil, errors.Wrap(err, "signature")
	}

	blockref, err := hexutil.Decode(rawTransaction.BlockRef)
	if err != nil {
		return nil, errors.Wrap(err, "blockRef")
	}
	var blkRef tx.BlockRef
	copy(blkRef[:], blockref[:])
	return builder.ChainTag(rawTransaction.ChainTag).
		GasPriceCoef(rawTransaction.GasPriceCoef).
		Gas(uint64(rawTransaction.Gas)).
		DependsOn(rawTransaction.DependsOn).
		BlockRef(blkRef).
		Nonce(uint64(rawTransaction.Nonce)).
		Build().
		WithSignature(sig), nil
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

//Receipt for json marshal
type Receipt struct {
	// gas used by this tx
	GasUsed math.HexOrDecimal64 `json:"gasUsed"`
	// the one who payed for gas
	GasPayer thor.Address `json:"gasPayer,string"`
	// if the tx reverted
	Reverted bool `json:"reverted"`
	// outputs of clauses in tx
	Outputs []*Output `json:"outputs,string"`
}

// Output output of clause execution.
type Output struct {
	// logs produced by the clause
	Logs []*ReceiptLog `json:"outputs,string"`
}

// ReceiptLog ReceiptLog.
type ReceiptLog struct {
	// address of the contract that generated the event
	Address thor.Address `json:"address,string"`
	// list of topics provided by the contract.
	Topics []thor.Hash `json:"topics,string"`
	// supplied by the contract, usually ABI-encoded
	Data string `json:"data"`
}

//ConvertReceipt convert a raw clause into a jason format clause
func convertReceipt(rece *tx.Receipt) *Receipt {
	receipt := &Receipt{
		GasUsed:  math.HexOrDecimal64(rece.GasUsed),
		GasPayer: rece.GasPayer,
		Reverted: rece.Reverted,
	}
	receipt.Outputs = make([]*Output, len(rece.Outputs))
	for i, output := range rece.Outputs {
		otp := &Output{make([]*ReceiptLog, len(output.Logs))}
		for j, log := range output.Logs {
			receiptLog := &ReceiptLog{
				Address: log.Address,
				Data:    hexutil.Encode(log.Data),
			}
			receiptLog.Topics = make([]thor.Hash, len(log.Topics))
			for k, topic := range log.Topics {
				receiptLog.Topics[k] = topic
			}
			otp.Logs[j] = receiptLog
		}
		receipt.Outputs[i] = otp
	}
	return receipt
}
