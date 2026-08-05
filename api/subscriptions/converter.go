// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api/convert"
	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func ConvertBlock(b *chain.ExtendedBlock) (*dto.BlockMessage, error) {
	header := b.Header()
	signer, err := header.Signer()
	if err != nil {
		return nil, err
	}

	txs := b.Transactions()
	txIDs := make([]thor.Bytes32, len(txs))
	for i, tx := range txs {
		txIDs[i] = tx.ID()
	}
	return &dto.BlockMessage{
		BlockBase:    convert.ConvertBlockBase(header, uint32(b.Size()), signer),
		Transactions: txIDs,
		Obsolete:     b.Obsolete,
	}, nil
}

func ConvertSubscriptionTransfer(
	header *block.Header,
	tx *tx.Transaction,
	clauseIndex uint32,
	transfer *tx.Transfer,
	obsolete bool,
) (*dto.TransferMessage, error) {
	origin, err := tx.Origin()
	if err != nil {
		return nil, err
	}

	return &dto.TransferMessage{
		FilteredTransfer: dto.FilteredTransfer{
			Sender:    transfer.Sender,
			Recipient: transfer.Recipient,
			Amount:    (*math.HexOrDecimal256)(transfer.Amount),
			Meta: dto.LogMeta{
				BlockID:        header.ID(),
				BlockNumber:    header.Number(),
				BlockTimestamp: header.Timestamp(),
				TxID:           tx.ID(),
				TxOrigin:       origin,
				ClauseIndex:    clauseIndex,
			},
		},
		Obsolete: obsolete,
	}, nil
}

func ConvertSubscriptionEvent(header *block.Header, tx *tx.Transaction, clauseIndex uint32, event *tx.Event, obsolete bool) (*dto.EventMessage, error) {
	signer, err := tx.Origin()
	if err != nil {
		return nil, err
	}
	return &dto.EventMessage{
		FilteredEvent: dto.FilteredEvent{
			Address: event.Address,
			Data:    hexutil.Encode(event.Data),
			Meta: dto.LogMeta{
				BlockID:        header.ID(),
				BlockNumber:    header.Number(),
				BlockTimestamp: header.Timestamp(),
				TxID:           tx.ID(),
				TxOrigin:       signer,
				ClauseIndex:    clauseIndex,
			},
			Topics: event.Topics,
		},
		Obsolete: obsolete,
	}, nil
}
