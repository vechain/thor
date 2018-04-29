package transfers

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
)

type FilteredTransfer struct {
	BlockID     thor.Bytes32          `json:"blockID"`
	Index       uint32                `json:"index"`
	BlockNumber uint32                `json:"blockNumber"`
	BlockTime   uint64                `json:"blockTime"`
	TxID        thor.Bytes32          `json:"txID"`
	TxOrigin    thor.Address          `json:"txOrigin"`
	Sender      thor.Address          `json:"sender"`
	Recipient   thor.Address          `json:"recipient"`
	Amount      *math.HexOrDecimal256 `json:"amount"`
}

func ConvertTransfer(transfer *logdb.Transfer) *FilteredTransfer {
	v := math.HexOrDecimal256(*transfer.Amount)
	return &FilteredTransfer{
		BlockID:     transfer.BlockID,
		Index:       transfer.Index,
		BlockNumber: transfer.BlockNumber,
		BlockTime:   transfer.BlockTime,
		TxID:        transfer.TxID,
		TxOrigin:    transfer.TxOrigin,
		Sender:      transfer.Sender,
		Recipient:   transfer.Recipient,
		Amount:      &v,
	}
}
