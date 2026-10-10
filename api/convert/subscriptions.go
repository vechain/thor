// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func ConvertBlock(b *chain.ExtendedBlock) (*api.BlockMessage, error) {
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
	return &api.BlockMessage{
		Number:        header.Number(),
		ID:            header.ID(),
		ParentID:      header.ParentID(),
		Timestamp:     header.Timestamp(),
		TotalScore:    header.TotalScore(),
		GasLimit:      header.GasLimit(),
		GasUsed:       header.GasUsed(),
		BaseFeePerGas: (*math.HexOrDecimal256)(header.BaseFee()),
		Beneficiary:   header.Beneficiary(),
		Signer:        signer,
		Size:          uint32(b.Size()),
		StateRoot:     header.StateRoot(),
		ReceiptsRoot:  header.ReceiptsRoot(),
		TxsRoot:       header.TxsRoot(),
		TxsFeatures:   uint32(header.TxsFeatures()),
		COM:           header.COM(),
		Transactions:  txIDs,
		Obsolete:      b.Obsolete,
	}, nil
}

func ConvertSubscriptionTransfer(
	header *block.Header,
	tx *tx.Transaction,
	clauseIndex uint32,
	transfer *tx.Transfer,
	obsolete bool,
) (*api.TransferMessage, error) {
	origin, err := tx.Origin()
	if err != nil {
		return nil, err
	}

	return &api.TransferMessage{
		Sender:    transfer.Sender,
		Recipient: transfer.Recipient,
		Amount:    (*math.HexOrDecimal256)(transfer.Amount),
		Meta: api.LogMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
			TxID:           tx.ID(),
			TxOrigin:       origin,
			ClauseIndex:    clauseIndex,
		},
		Obsolete: obsolete,
	}, nil
}

func ConvertSubscriptionEvent(header *block.Header, tx *tx.Transaction, clauseIndex uint32, event *tx.Event, obsolete bool) (*api.EventMessage, error) {
	signer, err := tx.Origin()
	if err != nil {
		return nil, err
	}
	return &api.EventMessage{
		Address: event.Address,
		Data:    hexutil.Encode(event.Data),
		Meta: api.LogMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
			TxID:           tx.ID(),
			TxOrigin:       signer,
			ClauseIndex:    clauseIndex,
		},
		Topics:   event.Topics,
		Obsolete: obsolete,
	}, nil
}
