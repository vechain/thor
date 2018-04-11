package transactions

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Clause for json marshal
type Clause struct {
	To    *thor.Address        `json:"to,string"`
	Value math.HexOrDecimal256 `json:"value,string"`
	Data  string               `json:"data"`
}

//Clauses array of clauses.
type Clauses []Clause

//ConvertClause convert a raw clause into a json format clause
func ConvertClause(c *tx.Clause) Clause {
	return Clause{
		c.To(),
		math.HexOrDecimal256(*c.Value()),
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

//Transaction transaction
type Transaction struct {
	Block        blockContext  `json:"block"`
	Size         uint32        `json:"size"`
	ChainTag     byte          `json:"chainTag"`
	ID           thor.Bytes32  `json:"id,string"`
	GasPriceCoef uint8         `json:"gasPriceCoef"`
	Gas          uint64        `json:"gas"`
	Origin       thor.Address  `json:"origin,string"`
	DependsOn    *thor.Bytes32 `json:"dependsOn,string"`
	Clauses      Clauses       `json:"clauses"`
}

//ConvertTransaction convert a raw transaction into a json format transaction
func ConvertTransaction(tx *tx.Transaction) (*Transaction, error) {
	//tx signer
	signer, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	cls := make(Clauses, len(tx.Clauses()))
	for i, c := range tx.Clauses() {
		cls[i] = ConvertClause(c)
	}
	t := &Transaction{
		ChainTag:     tx.ChainTag(),
		ID:           tx.ID(),
		Origin:       signer,
		Size:         uint32(tx.Size()),
		GasPriceCoef: tx.GasPriceCoef(),
		Gas:          tx.Gas(),
		Clauses:      cls,
	}
	if tx.DependsOn() != nil {
		t.DependsOn = tx.DependsOn()
	}
	return t, nil
}

type blockContext struct {
	ID        thor.Bytes32 `json:"id"`
	Number    uint32       `json:"number"`
	Timestamp uint64       `json:"timestamp"`
}

type txContext struct {
	ID     thor.Bytes32 `json:"id"`
	Origin thor.Address `json:"origin"`
}

//Receipt for json marshal
type Receipt struct {
	Block    blockContext          `json:"block"`
	Tx       txContext             `json:"tx"`
	GasUsed  math.HexOrDecimal64   `json:"gasUsed"`
	GasPayer thor.Address          `json:"gasPayer,string"`
	Reward   *math.HexOrDecimal256 `json:"reward,string"`
	Reverted bool                  `json:"reverted"`
	Outputs  []*Output             `json:"outputs,string"`
}

// Output output of clause execution.
type Output struct {
	ContractAddress *thor.Address `json:"contractAddress"`
	// logs produced by the clause
	Logs []*ReceiptLog `json:"logs,string"`
}

// ReceiptLog ReceiptLog.
type ReceiptLog struct {
	// address of the contract that generated the event
	Address thor.Address `json:"address,string"`
	// list of topics provided by the contract.
	Topics []thor.Bytes32 `json:"topics,string"`
	// supplied by the contract, usually ABI-encoded
	Data string `json:"data"`
}

//ConvertReceipt convert a raw clause into a jason format clause
func convertReceipt(rece *tx.Receipt, block *block.Block, tx *tx.Transaction) (*Receipt, error) {
	reward := math.HexOrDecimal256(*rece.Reward)
	signer, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	receipt := &Receipt{
		GasUsed:  math.HexOrDecimal64(rece.GasUsed),
		GasPayer: rece.GasPayer,
		Reward:   &reward,
		Reverted: rece.Reverted,
		Tx: txContext{
			tx.ID(),
			signer,
		},
		Block: blockContext{
			block.Header().ID(),
			block.Header().Number(),
			block.Header().Timestamp(),
		},
	}
	receipt.Outputs = make([]*Output, len(rece.Outputs))
	for i, output := range rece.Outputs {
		clause := tx.Clauses()[i]
		var contractAddr *thor.Address
		if clause.To() == nil {
			cAddr := thor.CreateContractAddress(tx.ID(), uint32(i), 0)
			contractAddr = &cAddr
		}
		otp := &Output{contractAddr, make([]*ReceiptLog, len(output.Logs))}
		for j, log := range output.Logs {
			receiptLog := &ReceiptLog{
				Address: log.Address,
				Data:    hexutil.Encode(log.Data),
			}
			receiptLog.Topics = make([]thor.Bytes32, len(log.Topics))
			for k, topic := range log.Topics {
				receiptLog.Topics[k] = topic
			}
			otp.Logs[j] = receiptLog
		}
		receipt.Outputs[i] = otp
	}
	return receipt, nil
}
