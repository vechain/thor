// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

type LogMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
	TxID           thor.Bytes32 `json:"txID"`
	TxOrigin       thor.Address `json:"txOrigin"`
	ClauseIndex    uint32       `json:"clauseIndex"`
	TxIndex        *uint32      `json:"txIndex,omitempty"`
	LogIndex       *uint32      `json:"logIndex,omitempty"`
}

type FilteredTransfer struct {
	Sender    thor.Address          `json:"sender"`
	Recipient thor.Address          `json:"recipient"`
	Amount    *math.HexOrDecimal256 `json:"amount"`
	Meta      LogMeta               `json:"meta"`
}

func convertTransfer(transfer *logdb.Transfer, addIndexes bool) *FilteredTransfer {
	v := math.HexOrDecimal256(*transfer.Amount)
	ft := &FilteredTransfer{
		Sender:    transfer.Sender,
		Recipient: transfer.Recipient,
		Amount:    &v,
		Meta: LogMeta{
			BlockID:        transfer.BlockID,
			BlockNumber:    transfer.BlockNumber,
			BlockTimestamp: transfer.BlockTime,
			TxID:           transfer.TxID,
			TxOrigin:       transfer.TxOrigin,
			ClauseIndex:    transfer.ClauseIndex,
		},
	}

	if addIndexes {
		ft.Meta.TxIndex = &transfer.TxIndex
		ft.Meta.LogIndex = &transfer.LogIndex
	}

	return ft
}

type TransferFilter struct {
	CriteriaSet []*logdb.TransferCriteria
	Range       *events.Range
	Options     *events.Options
	Order       logdb.Order //default asc
}
