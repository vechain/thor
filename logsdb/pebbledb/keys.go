// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"encoding/binary"

	"github.com/vechain/thor/v2/thor"
)

// Key prefixes for different data types
const (
	// Primary storage prefixes
	eventPrimaryPrefix    = "E"
	transferPrimaryPrefix = "T"

	// Event index prefixes
	eventAddrPrefix   = "EA"
	eventTopic0Prefix = "ET0"
	eventTopic1Prefix = "ET1"
	eventTopic2Prefix = "ET2"
	eventTopic3Prefix = "ET3"
	eventTopic4Prefix = "ET4"

	// Transfer index prefixes
	transferSenderPrefix    = "TS"
	transferRecipientPrefix = "TR"
	transferTxOriginPrefix  = "TO"

	// Dense sequence index prefixes
	eventSequencePrefix    = "ES"
	transferSequencePrefix = "TSX"
)

// Primary storage keys

func eventPrimaryKey(seq sequence) []byte {
	key := make([]byte, 0, 1+8)
	key = append(key, eventPrimaryPrefix...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func transferPrimaryKey(seq sequence) []byte {
	key := make([]byte, 0, 1+8)
	key = append(key, transferPrimaryPrefix...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

// Event index keys

func eventAddressKey(addr thor.Address, seq sequence) []byte {
	key := make([]byte, 0, 2+20+8)
	key = append(key, eventAddrPrefix...)
	key = append(key, addr[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func eventTopicKey(topicIndex int, topic thor.Bytes32, seq sequence) []byte {
	var prefix string
	switch topicIndex {
	case 0:
		prefix = eventTopic0Prefix
	case 1:
		prefix = eventTopic1Prefix
	case 2:
		prefix = eventTopic2Prefix
	case 3:
		prefix = eventTopic3Prefix
	case 4:
		prefix = eventTopic4Prefix
	default:
		return nil
	}

	key := make([]byte, 0, 3+32+8)
	key = append(key, prefix...)
	key = append(key, topic[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

// Transfer index keys

func transferSenderKey(addr thor.Address, seq sequence) []byte {
	key := make([]byte, 0, 2+20+8)
	key = append(key, transferSenderPrefix...)
	key = append(key, addr[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func transferRecipientKey(addr thor.Address, seq sequence) []byte {
	key := make([]byte, 0, 2+20+8)
	key = append(key, transferRecipientPrefix...)
	key = append(key, addr[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func transferTxOriginKey(addr thor.Address, seq sequence) []byte {
	key := make([]byte, 0, 2+20+8)
	key = append(key, transferTxOriginPrefix...)
	key = append(key, addr[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

// Dense sequence index keys

func eventSequenceKey(seq sequence) []byte {
	key := make([]byte, 0, 2+8)
	key = append(key, eventSequencePrefix...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func transferSequenceKey(seq sequence) []byte {
	key := make([]byte, 0, 3+8)
	key = append(key, transferSequencePrefix...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

// extractBlockNumberFromBlockID extracts block number from Thor blockID
// Thor blockIDs consistently encode the block number in bytes 0-4 (big-endian)
// This encoding is stable across all Thor networks and production chains
func extractBlockNumberFromBlockID(id thor.Bytes32) uint32 {
	// Thor-specific: block number is always stored in first 4 bytes
	// This is a stable, production-tested encoding used across all Thor chains
	return binary.BigEndian.Uint32(id[0:4])
}
