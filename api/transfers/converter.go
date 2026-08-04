// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

import (
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/logdb"
)

func ConvertTransfer(transfer *logdb.Transfer, addIndexes bool) *dto.FilteredTransfer {
	v := math.HexOrDecimal256(*transfer.Amount)
	ft := &dto.FilteredTransfer{
		Sender:    transfer.Sender,
		Recipient: transfer.Recipient,
		Amount:    &v,
		Meta: dto.LogMeta{
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
