// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
)

// EventRecord represents the stored event data (RLP encoded)
type EventRecord struct {
	BlockID     thor.Bytes32   `json:"blockID"`
	BlockNumber uint32         `json:"blockNumber"`
	BlockTime   uint64         `json:"blockTime"`
	TxID        thor.Bytes32   `json:"txID"`
	TxIndex     uint32         `json:"txIndex"`
	TxOrigin    thor.Address   `json:"txOrigin"`
	ClauseIndex uint32         `json:"clauseIndex"`
	LogIndex    uint32         `json:"logIndex"`
	Address     thor.Address   `json:"address"`
	Topics      []thor.Bytes32 `json:"topics"` // Variable length for RLP efficiency
	Data        []byte         `json:"data"`
}

// TransferRecord represents the stored transfer data (RLP encoded)
type TransferRecord struct {
	BlockID     thor.Bytes32 `json:"blockID"`
	BlockNumber uint32       `json:"blockNumber"`
	BlockTime   uint64       `json:"blockTime"`
	TxID        thor.Bytes32 `json:"txID"`
	TxIndex     uint32       `json:"txIndex"`
	TxOrigin    thor.Address `json:"txOrigin"`
	ClauseIndex uint32       `json:"clauseIndex"`
	LogIndex    uint32       `json:"logIndex"`
	Sender      thor.Address `json:"sender"`
	Recipient   thor.Address `json:"recipient"`
	Amount      *big.Int     `json:"amount"`
}

// RLPEncode encodes the EventRecord using RLP
func (er *EventRecord) RLPEncode() ([]byte, error) {
	return rlp.EncodeToBytes(er)
}

// RLPDecode decodes RLP data into EventRecord
func (er *EventRecord) RLPDecode(data []byte) error {
	return rlp.DecodeBytes(data, er)
}

// ToLogDBEvent converts EventRecord to logsdb.Event
func (er *EventRecord) ToLogDBEvent() *logsdb.Event {
	// Convert []thor.Bytes32 back to [5]*thor.Bytes32
	var topics [5]*thor.Bytes32
	for i := 0; i < 5 && i < len(er.Topics); i++ {
		topics[i] = &er.Topics[i]
	}

	return &logsdb.Event{
		BlockID:     er.BlockID,
		BlockNumber: er.BlockNumber,
		BlockTime:   er.BlockTime,
		TxID:        er.TxID,
		TxIndex:     er.TxIndex,
		TxOrigin:    er.TxOrigin,
		ClauseIndex: er.ClauseIndex,
		LogIndex:    er.LogIndex,
		Address:     er.Address,
		Topics:      topics,
		Data:        er.Data,
	}
}

// NewEventRecord creates EventRecord from logsdb.Event
func NewEventRecord(event *logsdb.Event) *EventRecord {
	// Convert [5]*thor.Bytes32 to []thor.Bytes32 for RLP efficiency
	var topics []thor.Bytes32
	for _, topic := range event.Topics {
		if topic != nil {
			topics = append(topics, *topic)
		}
	}

	return &EventRecord{
		BlockID:     event.BlockID,
		BlockNumber: event.BlockNumber,
		BlockTime:   event.BlockTime,
		TxID:        event.TxID,
		TxIndex:     event.TxIndex,
		TxOrigin:    event.TxOrigin,
		ClauseIndex: event.ClauseIndex,
		LogIndex:    event.LogIndex,
		Address:     event.Address,
		Topics:      topics,
		Data:        event.Data,
	}
}

// RLPEncode encodes the TransferRecord using RLP
func (tr *TransferRecord) RLPEncode() ([]byte, error) {
	return rlp.EncodeToBytes(tr)
}

// RLPDecode decodes RLP data into TransferRecord
func (tr *TransferRecord) RLPDecode(data []byte) error {
	return rlp.DecodeBytes(data, tr)
}

// ToLogDBTransfer converts TransferRecord to logsdb.Transfer
func (tr *TransferRecord) ToLogDBTransfer() *logsdb.Transfer {
	return &logsdb.Transfer{
		BlockID:     tr.BlockID,
		BlockNumber: tr.BlockNumber,
		BlockTime:   tr.BlockTime,
		TxID:        tr.TxID,
		TxIndex:     tr.TxIndex,
		TxOrigin:    tr.TxOrigin,
		ClauseIndex: tr.ClauseIndex,
		LogIndex:    tr.LogIndex,
		Sender:      tr.Sender,
		Recipient:   tr.Recipient,
		Amount:      tr.Amount,
	}
}

// NewTransferRecord creates TransferRecord from logsdb.Transfer
func NewTransferRecord(transfer *logsdb.Transfer) *TransferRecord {
	return &TransferRecord{
		BlockID:     transfer.BlockID,
		BlockNumber: transfer.BlockNumber,
		BlockTime:   transfer.BlockTime,
		TxID:        transfer.TxID,
		TxIndex:     transfer.TxIndex,
		TxOrigin:    transfer.TxOrigin,
		ClauseIndex: transfer.ClauseIndex,
		LogIndex:    transfer.LogIndex,
		Sender:      transfer.Sender,
		Recipient:   transfer.Recipient,
		Amount:      transfer.Amount,
	}
}
