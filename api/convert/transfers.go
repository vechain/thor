// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/logdb"
)

func ConvertTransfer(transfer *logdb.Transfer, addIndexes bool) *api.FilteredTransfer {
	v := math.HexOrDecimal256(*transfer.Amount)
	ft := &api.FilteredTransfer{
		Sender:    transfer.Sender,
		Recipient: transfer.Recipient,
		Amount:    &v,
		Meta: api.LogMeta{
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

// ConvertTransferCriteria maps request criteria to logdb criteria, keeping nil
// as nil and an empty set as empty.
func ConvertTransferCriteria(cs []*api.TransferCriteria) []*logdb.TransferCriteria {
	if cs == nil {
		return nil
	}
	criteria := make([]*logdb.TransferCriteria, len(cs))
	for i, c := range cs {
		criteria[i] = &logdb.TransferCriteria{TxOrigin: c.TxOrigin, Sender: c.Sender, Recipient: c.Recipient}
	}
	return criteria
}
