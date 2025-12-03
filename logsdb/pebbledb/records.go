// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"math/big"

	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
)

// EventRecord represents the stored event data (binary encoded)
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
	Topics      []thor.Bytes32 `json:"topics"` // Variable length
	Data        []byte         `json:"data"`
}

// TransferRecord represents the stored transfer data (binary encoded)
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

// Encode encodes the EventRecord using binary format
func (er *EventRecord) Encode() ([]byte, error) {
	return EncodeEventRecord(er)
}

// Decode decodes binary data into EventRecord
func (er *EventRecord) Decode(data []byte) error {
	decoded, err := DecodeEventRecord(data)
	if err != nil {
		return err
	}
	*er = *decoded
	return nil
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
	// Convert [5]*thor.Bytes32 to []thor.Bytes32
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

// Encode encodes the TransferRecord using binary format
func (tr *TransferRecord) Encode() ([]byte, error) {
	return EncodeTransferRecord(tr)
}

// Decode decodes binary data into TransferRecord
func (tr *TransferRecord) Decode(data []byte) error {
	decoded, err := DecodeTransferRecord(data)
	if err != nil {
		return err
	}
	*tr = *decoded
	return nil
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

// reset clears EventRecord for reuse in object pool
func (er *EventRecord) reset() {
	// Reset all fields but reuse slices if possible
	er.BlockID = thor.Bytes32{}
	er.BlockNumber = 0
	er.BlockTime = 0
	er.TxID = thor.Bytes32{}
	er.TxIndex = 0
	er.TxOrigin = thor.Address{}
	er.ClauseIndex = 0
	er.LogIndex = 0
	er.Address = thor.Address{}
	// Reset slice length but keep capacity for reuse
	er.Topics = er.Topics[:0]
	er.Data = er.Data[:0]
}

// reset clears TransferRecord for reuse in object pool  
func (tr *TransferRecord) reset() {
	tr.BlockID = thor.Bytes32{}
	tr.BlockNumber = 0
	tr.BlockTime = 0
	tr.TxID = thor.Bytes32{}
	tr.TxIndex = 0
	tr.TxOrigin = thor.Address{}
	tr.ClauseIndex = 0
	tr.Sender = thor.Address{}
	tr.Recipient = thor.Address{}
	tr.Amount = nil
}
