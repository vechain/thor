// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/api/transactions"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
)

type FilteredTransfer struct {
	Sender    thor.Address              `json:"sender"`
	Recipient thor.Address              `json:"recipient"`
	Amount    *math.HexOrDecimal256     `json:"amount"`
	Block     transactions.BlockContext `json:"block"`
	Tx        transactions.TxContext    `json:"tx"`
}

func ConvertTransfer(transfer *logdb.Transfer) *FilteredTransfer {
	v := math.HexOrDecimal256(*transfer.Amount)
	return &FilteredTransfer{
		Sender:    transfer.Sender,
		Recipient: transfer.Recipient,
		Amount:    &v,
		Block: transactions.BlockContext{
			ID:        transfer.BlockID,
			Number:    transfer.BlockNumber,
			Timestamp: transfer.BlockTime,
		},
		Tx: transactions.TxContext{
			ID:     transfer.TxID,
			Origin: transfer.TxOrigin,
		},
	}
}
