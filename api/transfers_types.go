// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

type FilteredTransfer struct {
	Sender    thor.Address          `json:"sender"`
	Recipient thor.Address          `json:"recipient"`
	Amount    *math.HexOrDecimal256 `json:"amount"`
	Meta      LogMeta               `json:"meta"`
}

// TransferCriteria has the same fields as logdb.TransferCriteria and, like it, no
// json tags: keys are matched case-insensitively and encoded by field name.
type TransferCriteria struct {
	TxOrigin  *thor.Address // who send transaction
	Sender    *thor.Address // who transferred tokens
	Recipient *thor.Address // who received tokens
}

type TransferFilter struct {
	CriteriaSet []*TransferCriteria `json:"criteriaSet,omitempty"`
	Range       *Range              `json:"range,omitempty"`
	Options     *Options            `json:"options,omitempty"`
	Order       Order               `json:"order,omitempty"`
}

// ConvertTransferCriteria maps request criteria to logdb criteria, keeping nil
// as nil and an empty set as empty.
func ConvertTransferCriteria(cs []*TransferCriteria) []*logdb.TransferCriteria {
	if cs == nil {
		return nil
	}
	criteria := make([]*logdb.TransferCriteria, len(cs))
	for i, c := range cs {
		criteria[i] = &logdb.TransferCriteria{TxOrigin: c.TxOrigin, Sender: c.Sender, Recipient: c.Recipient}
	}
	return criteria
}

func ConvertTransfer(transfer *logdb.Transfer, addIndexes bool) *FilteredTransfer {
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
