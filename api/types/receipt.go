package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//Receipt for json marshal
type Receipt struct {
	// gas used by this tx
	GasUsed math.HexOrDecimal64 `json:"gasUsed"`
	// the one who payed for gas
	GasPayer string `json:"gasPayer,string"`
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
func ConvertReceipt(rece *tx.Receipt) *Receipt {
	receipt := &Receipt{
		GasUsed:  math.HexOrDecimal64(rece.GasUsed),
		GasPayer: rece.GasPayer.String(),
		Reverted: rece.Reverted,
	}
	if len(rece.Outputs) > 0 {
		receipt.Outputs = make([]*Output, len(rece.Outputs))
		for _, output := range rece.Outputs {
			logs := make([]*ReceiptLog, len(output.Logs))
			for _, log := range output.Logs {
				receiptLog := &ReceiptLog{
					Address: log.Address,
					Data:    hexutil.Encode(log.Data),
				}
				receiptLog.Topics = make([]thor.Hash, len(log.Topics))
				for k, topic := range log.Topics {
					receiptLog.Topics[k] = topic
				}
				logs = append(logs, receiptLog)
			}
			receipt.Outputs = append(receipt.Outputs, &Output{logs})
		}
	}
	return receipt
}
