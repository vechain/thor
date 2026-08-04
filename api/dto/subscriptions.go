// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package dto

import (
	"github.com/ethereum/go-ethereum/common/math"

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

// TransferMessage transfer piped by websocket
type TransferMessage struct {
	Sender    thor.Address          `json:"sender"`
	Recipient thor.Address          `json:"recipient"`
	Amount    *math.HexOrDecimal256 `json:"amount"`
	Meta      LogMeta               `json:"meta"`
	Obsolete  bool                  `json:"obsolete"`
}

// EventMessage event piped by websocket
type EventMessage struct {
	Address  thor.Address   `json:"address"`
	Topics   []thor.Bytes32 `json:"topics"`
	Data     string         `json:"data"`
	Meta     LogMeta        `json:"meta"`
	Obsolete bool           `json:"obsolete"`
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

// Match returns whether event matches filter
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

// Match returns whether transfer matches filter
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
