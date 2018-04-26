package transfers

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
)

type FilteredTransfer struct {
	BlockID     thor.Bytes32          `json:"blockID"`
	Index       uint32                `json:"index"`
	BlockNumber uint32                `json:"blockNumber"`
	BlockTime   uint64                `json:"blockTime"`
	TxID        thor.Bytes32          `json:"txID"`
	TxOrigin    thor.Address          `json:"txOrigin"`
	From        thor.Address          `json:"from"`
	To          thor.Address          `json:"to"`
	Value       *math.HexOrDecimal256 `json:"value"`
}

func ConvertTransfer(transfer *transferdb.Transfer) *FilteredTransfer {
	v := math.HexOrDecimal256(*transfer.Value)
	return &FilteredTransfer{
		BlockID:     transfer.BlockID,
		Index:       transfer.Index,
		BlockNumber: transfer.BlockNumber,
		BlockTime:   transfer.BlockTime,
		TxID:        transfer.TxID,
		TxOrigin:    transfer.TxOrigin,
		From:        transfer.From,
		To:          transfer.To,
		Value:       &v,
	}
}
