// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

// LogsDB Binary Encoding
// Fast, compact, versioned binary encoding for EventRecord and TransferRecord

// Flag constants for the flags byte
const (
	FlagIsEvent   uint8 = 1 << 0 // 1=Event, 0=Transfer
	FlagHasTopics uint8 = 1 << 1 // Topics section present
	FlagHasData   uint8 = 1 << 2 // Data section present
	// Bits 3-7 reserved for future use
)

// Field mask constants for the fieldMask uint32
const (
	FieldBlockID   uint32 = 1 << 0
	FieldBlockNum  uint32 = 1 << 1
	FieldBlockTime uint32 = 1 << 2
	FieldTxID      uint32 = 1 << 3
	FieldTxIndex   uint32 = 1 << 4
	FieldTxOrigin  uint32 = 1 << 5
	FieldClauseIdx uint32 = 1 << 6
	FieldLogIdx    uint32 = 1 << 7
	FieldAddress   uint32 = 1 << 8  // Event only
	FieldSender    uint32 = 1 << 9  // Transfer only
	FieldRecipient uint32 = 1 << 10 // Transfer only
	FieldAmount    uint32 = 1 << 11 // Transfer only
	// Bits 12+ reserved for future fields
)

// Required field masks for strict validation
const (
	EventRequiredFields = FieldBlockID | FieldBlockNum | FieldBlockTime | FieldTxID |
		FieldTxIndex | FieldTxOrigin | FieldClauseIdx | FieldLogIdx |
		FieldAddress

	TransferRequiredFields = FieldBlockID | FieldBlockNum | FieldBlockTime | FieldTxID |
		FieldTxIndex | FieldTxOrigin | FieldClauseIdx | FieldLogIdx |
		FieldSender | FieldRecipient | FieldAmount
)

// Errors
var (
	ErrCorruptEncoding    = errors.New("corrupt encoding data")
	ErrInvalidTopicsCount = errors.New("invalid topics count > 5")
)

// Size constants for fixed-size fields
const (
	HeaderSize       = 5                                // flags(1) + fieldMask(4)
	CommonFieldsSize = 32 + 4 + 8 + 32 + 4 + 20 + 4 + 4 // 108 bytes
	AddressSize      = 20
	SenderSize       = 20
	RecipientSize    = 20
	AmountSize       = 32
	TopicSize        = 32
)

// EncodeEventRecord encodes an EventRecord using binary v1 format
func EncodeEventRecord(r *EventRecord) ([]byte, error) {
	if r == nil {
		return nil, ErrCorruptEncoding
	}

	// Estimate buffer size to minimize allocations
	size := estimateEventRecordSize(r)
	buf := make([]byte, size)
	offset := 0

	// Write header
	flags := FlagIsEvent
	if len(r.Topics) > 0 {
		flags |= FlagHasTopics
	}
	if len(r.Data) > 0 {
		flags |= FlagHasData
	}

	offset = writeHeader(buf, offset, flags, buildEventFieldMask())

	// Write fixed fields
	offset = writeEventFixedFields(buf, offset, r)

	// Write variable fields
	if flags&FlagHasTopics != 0 {
		offset = writeTopicsSection(buf, offset, r.Topics)
	}
	if flags&FlagHasData != 0 {
		offset = writeDataSection(buf, offset, r.Data)
	}

	// Return exact-sized slice
	return buf[:offset], nil
}

// DecodeEventRecord decodes an EventRecord from binary format
func DecodeEventRecord(b []byte) (*EventRecord, error) {
	return decodeEventRecordBinary(b)
}

// EncodeTransferRecord encodes a TransferRecord using binary v1 format
func EncodeTransferRecord(r *TransferRecord) ([]byte, error) {
	if r == nil {
		return nil, ErrCorruptEncoding
	}

	// Estimate buffer size to minimize allocations
	size := estimateTransferRecordSize(r)
	buf := make([]byte, size)
	offset := 0

	// Write header (transfers never have topics or data)
	flags := uint8(0) // FlagIsEvent = 0 for transfers
	offset = writeHeader(buf, offset, flags, buildTransferFieldMask())

	// Write fixed fields
	offset = writeTransferFixedFields(buf, offset, r)

	// Return exact-sized slice
	return buf[:offset], nil
}

// DecodeTransferRecord decodes a TransferRecord from binary format
func DecodeTransferRecord(b []byte) (*TransferRecord, error) {
	return decodeTransferRecordBinary(b)
}

// Size estimation functions
func estimateEventRecordSize(r *EventRecord) int {
	size := HeaderSize + CommonFieldsSize + AddressSize

	if len(r.Topics) > 0 {
		size += 1 + len(r.Topics)*TopicSize // topicsCount + topics
	}

	if len(r.Data) > 0 {
		size += binary.MaxVarintLen64 + len(r.Data) // dataLen + data
	}

	return size
}

func estimateTransferRecordSize(r *TransferRecord) int {
	return HeaderSize + CommonFieldsSize + SenderSize + RecipientSize + AmountSize
}

// Field mask builders
func buildEventFieldMask() uint32 {
	return EventRequiredFields
}

func buildTransferFieldMask() uint32 {
	return TransferRequiredFields
}

// Header writing
func writeHeader(buf []byte, offset int, flags uint8, fieldMask uint32) int {
	buf[offset] = flags
	binary.BigEndian.PutUint32(buf[offset+1:], fieldMask)
	return offset + HeaderSize
}

// Fixed fields writing - separate functions for each record type (no reflection)
func writeEventFixedFields(buf []byte, offset int, r *EventRecord) int {
	// Common fields (same order as TransferRecord)
	copy(buf[offset:], r.BlockID[:])
	offset += 32
	binary.BigEndian.PutUint32(buf[offset:], r.BlockNumber)
	offset += 4
	binary.BigEndian.PutUint64(buf[offset:], r.BlockTime)
	offset += 8
	copy(buf[offset:], r.TxID[:])
	offset += 32
	binary.BigEndian.PutUint32(buf[offset:], r.TxIndex)
	offset += 4
	copy(buf[offset:], r.TxOrigin[:])
	offset += 20
	binary.BigEndian.PutUint32(buf[offset:], r.ClauseIndex)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], r.LogIndex)
	offset += 4

	// Event-specific fields
	copy(buf[offset:], r.Address[:])
	offset += 20

	return offset
}

func writeTransferFixedFields(buf []byte, offset int, r *TransferRecord) int {
	// Common fields (same order as EventRecord)
	copy(buf[offset:], r.BlockID[:])
	offset += 32
	binary.BigEndian.PutUint32(buf[offset:], r.BlockNumber)
	offset += 4
	binary.BigEndian.PutUint64(buf[offset:], r.BlockTime)
	offset += 8
	copy(buf[offset:], r.TxID[:])
	offset += 32
	binary.BigEndian.PutUint32(buf[offset:], r.TxIndex)
	offset += 4
	copy(buf[offset:], r.TxOrigin[:])
	offset += 20
	binary.BigEndian.PutUint32(buf[offset:], r.ClauseIndex)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], r.LogIndex)
	offset += 4

	// Transfer-specific fields
	copy(buf[offset:], r.Sender[:])
	offset += 20
	copy(buf[offset:], r.Recipient[:])
	offset += 20
	// Encode big.Int as 32-byte big-endian
	amountBytes := make([]byte, 32)
	r.Amount.FillBytes(amountBytes)
	copy(buf[offset:], amountBytes)
	offset += 32

	return offset
}

// Variable sections writing
func writeTopicsSection(buf []byte, offset int, topics []thor.Bytes32) int {
	if len(topics) > 5 {
		// This should never happen with valid data, but be defensive
		topics = topics[:5]
	}

	buf[offset] = uint8(len(topics))
	offset++

	for _, topic := range topics {
		copy(buf[offset:], topic[:])
		offset += 32
	}

	return offset
}

func writeDataSection(buf []byte, offset int, data []byte) int {
	// Write data length as uvarint
	n := binary.PutUvarint(buf[offset:], uint64(len(data)))
	offset += n

	// Write data bytes
	copy(buf[offset:], data)
	offset += len(data)

	return offset
}

// Binary decoding functions
func decodeEventRecordBinary(b []byte) (*EventRecord, error) {
	// Read and validate header
	flags, fieldMask, offset, err := readHeader(b)
	if err != nil {
		return nil, err
	}

	if flags&FlagIsEvent == 0 {
		return nil, ErrCorruptEncoding // Not an event record
	}

	if err := validateFieldMask(fieldMask, true); err != nil {
		return nil, err
	}

	// Read fixed fields
	record := &EventRecord{}
	offset, err = readEventFixedFields(b, offset, record)
	if err != nil {
		return nil, err
	}

	// Read variable sections
	if flags&FlagHasTopics != 0 {
		offset, err = readTopicsSection(b, offset, record)
		if err != nil {
			return nil, err
		}
	}

	if flags&FlagHasData != 0 {
		_, err = readDataSection(b, offset, record)
		if err != nil {
			return nil, err
		}
	}

	return record, nil
}

func decodeTransferRecordBinary(b []byte) (*TransferRecord, error) {
	// Read and validate header
	flags, fieldMask, offset, err := readHeader(b)
	if err != nil {
		return nil, err
	}

	if flags&FlagIsEvent != 0 {
		return nil, ErrCorruptEncoding // Not a transfer record
	}

	if err := validateFieldMask(fieldMask, false); err != nil {
		return nil, err
	}

	// Read fixed fields
	record := &TransferRecord{}
	_, err = readTransferFixedFields(b, offset, record)
	if err != nil {
		return nil, err
	}

	return record, nil
}

// Header reading
func readHeader(b []byte) (flags uint8, fieldMask uint32, offset int, err error) {
	if len(b) < HeaderSize {
		return 0, 0, 0, ErrCorruptEncoding
	}

	flags = b[0]
	fieldMask = binary.BigEndian.Uint32(b[1:5])
	offset = HeaderSize

	return flags, fieldMask, offset, nil
}

// Fixed fields reading - separate functions for each record type (no reflection)
func readEventFixedFields(b []byte, offset int, r *EventRecord) (newOffset int, err error) {
	required := CommonFieldsSize + AddressSize
	if len(b) < offset+required {
		return 0, ErrCorruptEncoding
	}

	// Common fields
	copy(r.BlockID[:], b[offset:offset+32])
	offset += 32
	r.BlockNumber = binary.BigEndian.Uint32(b[offset:])
	offset += 4
	r.BlockTime = binary.BigEndian.Uint64(b[offset:])
	offset += 8
	copy(r.TxID[:], b[offset:offset+32])
	offset += 32
	r.TxIndex = binary.BigEndian.Uint32(b[offset:])
	offset += 4
	copy(r.TxOrigin[:], b[offset:offset+20])
	offset += 20
	r.ClauseIndex = binary.BigEndian.Uint32(b[offset:])
	offset += 4
	r.LogIndex = binary.BigEndian.Uint32(b[offset:])
	offset += 4

	// Event-specific fields
	copy(r.Address[:], b[offset:offset+20])
	offset += 20

	return offset, nil
}

func readTransferFixedFields(b []byte, offset int, r *TransferRecord) (newOffset int, err error) {
	required := CommonFieldsSize + SenderSize + RecipientSize + AmountSize
	if len(b) < offset+required {
		return 0, ErrCorruptEncoding
	}

	// Common fields
	copy(r.BlockID[:], b[offset:offset+32])
	offset += 32
	r.BlockNumber = binary.BigEndian.Uint32(b[offset:])
	offset += 4
	r.BlockTime = binary.BigEndian.Uint64(b[offset:])
	offset += 8
	copy(r.TxID[:], b[offset:offset+32])
	offset += 32
	r.TxIndex = binary.BigEndian.Uint32(b[offset:])
	offset += 4
	copy(r.TxOrigin[:], b[offset:offset+20])
	offset += 20
	r.ClauseIndex = binary.BigEndian.Uint32(b[offset:])
	offset += 4
	r.LogIndex = binary.BigEndian.Uint32(b[offset:])
	offset += 4

	// Transfer-specific fields
	copy(r.Sender[:], b[offset:offset+20])
	offset += 20
	copy(r.Recipient[:], b[offset:offset+20])
	offset += 20
	// Decode 32-byte big-endian to big.Int
	r.Amount = new(big.Int)
	r.Amount.SetBytes(b[offset : offset+32])
	offset += 32

	return offset, nil
}

// Variable sections reading
func readTopicsSection(b []byte, offset int, r *EventRecord) (newOffset int, err error) {
	if len(b) <= offset {
		return 0, ErrCorruptEncoding
	}

	topicsCount := uint8(b[offset])
	offset++

	if topicsCount > 5 {
		return 0, ErrInvalidTopicsCount
	}

	required := int(topicsCount) * TopicSize
	if len(b) < offset+required {
		return 0, ErrCorruptEncoding
	}

	if topicsCount > 0 {
		// Reuse existing slice if possible, otherwise allocate new one
		if cap(r.Topics) >= int(topicsCount) {
			r.Topics = r.Topics[:topicsCount]
		} else {
			r.Topics = make([]thor.Bytes32, topicsCount)
		}
		for i := uint8(0); i < topicsCount; i++ {
			copy(r.Topics[i][:], b[offset:offset+32])
			offset += 32
		}
	}

	return offset, nil
}

func readDataSection(b []byte, offset int, r *EventRecord) (newOffset int, err error) {
	if len(b) <= offset {
		return 0, ErrCorruptEncoding
	}

	// Read data length as uvarint
	dataLen, n := binary.Uvarint(b[offset:])
	if n <= 0 {
		return 0, ErrCorruptEncoding
	}
	offset += n

	if len(b) < offset+int(dataLen) {
		return 0, ErrCorruptEncoding
	}

	if dataLen > 0 {
		// Reuse existing slice if possible, otherwise allocate new one
		if cap(r.Data) >= int(dataLen) {
			r.Data = r.Data[:dataLen]
		} else {
			r.Data = make([]byte, dataLen)
		}
		copy(r.Data, b[offset:offset+int(dataLen)])
		offset += int(dataLen)
	}

	return offset, nil
}

// Field mask validation
func validateFieldMask(fieldMask uint32, isEvent bool) error {
	required := TransferRequiredFields
	if isEvent {
		required = EventRequiredFields
	}

	if (fieldMask & required) != required {
		return ErrCorruptEncoding
	}

	// Unknown bits ≥ 12 are ignored (future compatibility)
	return nil
}
