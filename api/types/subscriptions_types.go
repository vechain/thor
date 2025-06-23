// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package types

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// BlockMessage block piped by websocket
type BlockMessage struct {
	Number        uint32                `json:"number"`
	ID            thor.Bytes32          `json:"id"`
	Size          uint32                `json:"size"`
	ParentID      thor.Bytes32          `json:"parentID"`
	Timestamp     uint64                `json:"timestamp"`
	GasLimit      uint64                `json:"gasLimit"`
	Beneficiary   thor.Address          `json:"beneficiary"`
	GasUsed       uint64                `json:"gasUsed"`
	BaseFeePerGas *math.HexOrDecimal256 `json:"baseFeePerGas,omitempty"`
	TotalScore    uint64                `json:"totalScore"`
	TxsRoot       thor.Bytes32          `json:"txsRoot"`
	TxsFeatures   uint32                `json:"txsFeatures"`
	StateRoot     thor.Bytes32          `json:"stateRoot"`
	ReceiptsRoot  thor.Bytes32          `json:"receiptsRoot"`
	COM           bool                  `json:"com"`
	Signer        thor.Address          `json:"signer"`
	Transactions  []thor.Bytes32        `json:"transactions"`
	Obsolete      bool                  `json:"obsolete"`
}

func ConvertBlock(b *chain.ExtendedBlock) (*BlockMessage, error) {
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
	return &BlockMessage{
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

// TransferMessage transfer piped by websocket
type TransferMessage struct {
	Sender    thor.Address          `json:"sender"`
	Recipient thor.Address          `json:"recipient"`
	Amount    *math.HexOrDecimal256 `json:"amount"`
	Meta      LogMeta               `json:"meta"`
	Obsolete  bool                  `json:"obsolete"`
}

func ConvertSubscriptionTransfer(header *block.Header, tx *tx.Transaction, clauseIndex uint32, transfer *tx.Transfer, obsolete bool) (*TransferMessage, error) {
	origin, err := tx.Origin()
	if err != nil {
		return nil, err
	}

	return &TransferMessage{
		Sender:    transfer.Sender,
		Recipient: transfer.Recipient,
		Amount:    (*math.HexOrDecimal256)(transfer.Amount),
		Meta: LogMeta{
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

// EventMessage event piped by websocket
type EventMessage struct {
	Address  thor.Address   `json:"address"`
	Topics   []thor.Bytes32 `json:"topics"`
	Data     string         `json:"data"`
	Meta     LogMeta        `json:"meta"`
	Obsolete bool           `json:"obsolete"`
}

func ConvertSubscriptionEvent(header *block.Header, tx *tx.Transaction, clauseIndex uint32, event *tx.Event, obsolete bool) (*EventMessage, error) {
	signer, err := tx.Origin()
	if err != nil {
		return nil, err
	}
	return &EventMessage{
		Address: event.Address,
		Data:    hexutil.Encode(event.Data),
		Meta: LogMeta{
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

// SubscriptionEventFilter contains options for contract event filtering.
type SubscriptionEventFilter struct {
	Address *thor.Address // restricts matches to events created by specific contracts
	Topic0  *thor.Bytes32
	Topic1  *thor.Bytes32
	Topic2  *thor.Bytes32
	Topic3  *thor.Bytes32
	Topic4  *thor.Bytes32
}

// Match returs whether event matches filter
func (ef *SubscriptionEventFilter) Match(event *tx.Event) bool {
	if (ef.Address != nil) && (*ef.Address != event.Address) {
		return false
	}

	matchTopic := func(topic *thor.Bytes32, index int) bool {
		if topic != nil {
			if len(event.Topics) <= index {
				return false
			}

			if *topic != event.Topics[index] {
				return false
			}
		}
		return true
	}

	return matchTopic(ef.Topic0, 0) &&
		matchTopic(ef.Topic1, 1) &&
		matchTopic(ef.Topic2, 2) &&
		matchTopic(ef.Topic3, 3) &&
		matchTopic(ef.Topic4, 4)
}

// SubscriptionTransferFilter contains options for contract transfer filtering.
type SubscriptionTransferFilter struct {
	TxOrigin  *thor.Address // who send transaction
	Sender    *thor.Address // who transferred tokens
	Recipient *thor.Address // who received tokens
}

// Match returs whether transfer matches filter
func (tf *SubscriptionTransferFilter) Match(transfer *tx.Transfer, origin thor.Address) bool {
	if (tf.TxOrigin != nil) && (*tf.TxOrigin != origin) {
		return false
	}

	if (tf.Sender != nil) && (*tf.Sender != transfer.Sender) {
		return false
	}

	if (tf.Recipient != nil) && (*tf.Recipient != transfer.Recipient) {
		return false
	}
	return true
}

type BeatMessage struct {
	Number      uint32       `json:"number"`
	ID          thor.Bytes32 `json:"id"`
	ParentID    thor.Bytes32 `json:"parentID"`
	Timestamp   uint64       `json:"timestamp"`
	TxsFeatures uint32       `json:"txsFeatures"`
	Bloom       string       `json:"bloom"`
	K           uint32       `json:"k"`
	Obsolete    bool         `json:"obsolete"`
}

type Beat2Message struct {
	Number        uint32                `json:"number"`
	ID            thor.Bytes32          `json:"id"`
	ParentID      thor.Bytes32          `json:"parentID"`
	Timestamp     uint64                `json:"timestamp"`
	TxsFeatures   uint32                `json:"txsFeatures"`
	BaseFeePerGas *math.HexOrDecimal256 `json:"baseFeePerGas,omitempty"`
	GasLimit      uint64                `json:"gasLimit"`
	Bloom         string                `json:"bloom"`
	K             uint8                 `json:"k"`
	Obsolete      bool                  `json:"obsolete"`
}

type PendingTxIDMessage struct {
	ID thor.Bytes32 `json:"id"`
}
