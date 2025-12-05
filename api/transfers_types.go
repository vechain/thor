// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
)

type FilteredTransfer struct {
	Sender    thor.Address          `json:"sender"`
	Recipient thor.Address          `json:"recipient"`
	Amount    *math.HexOrDecimal256 `json:"amount"`
	Meta      LogMeta               `json:"meta"`
}

type TransferFilter struct {
	CriteriaSet []*logsdb.TransferCriteria `json:"criteriaSet,omitempty"`
	Range       *Range                     `json:"range,omitempty"`
	Options     *Options                   `json:"options,omitempty"`
	Order       logsdb.Order               `json:"order,omitempty"`
}

func ConvertTransfer(transfer *logsdb.Transfer, addIndexes bool) *FilteredTransfer {
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
