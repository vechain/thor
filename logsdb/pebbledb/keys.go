// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
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
)

// Primary storage keys

func eventPrimaryKey(seq sequence) []byte {
	key := make([]byte, 0, 1+8)
	key = append(key, []byte(eventPrimaryPrefix)...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func transferPrimaryKey(seq sequence) []byte {
	key := make([]byte, 0, 1+8)
	key = append(key, []byte(transferPrimaryPrefix)...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

// Event index keys

func eventAddressKey(addr thor.Address, seq sequence) []byte {
	key := make([]byte, 0, 2+20+8)
	key = append(key, []byte(eventAddrPrefix)...)
	key = append(key, addr[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func eventTopicKey(topicIndex int, topic thor.Bytes32, seq sequence) []byte {
	prefixes := []string{eventTopic0Prefix, eventTopic1Prefix, eventTopic2Prefix, eventTopic3Prefix, eventTopic4Prefix}
	if topicIndex < 0 || topicIndex >= len(prefixes) {
		return nil
	}

	key := make([]byte, 0, 3+32+8)
	key = append(key, []byte(prefixes[topicIndex])...)
	key = append(key, topic[:]...) // Full 32 bytes - no leading zero compression
	key = append(key, seq.BigEndianBytes()...)
	return key
}

// Transfer index keys

func transferSenderKey(addr thor.Address, seq sequence) []byte {
	key := make([]byte, 0, 2+20+8)
	key = append(key, []byte(transferSenderPrefix)...)
	key = append(key, addr[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func transferRecipientKey(addr thor.Address, seq sequence) []byte {
	key := make([]byte, 0, 2+20+8)
	key = append(key, []byte(transferRecipientPrefix)...)
	key = append(key, addr[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

func transferTxOriginKey(addr thor.Address, seq sequence) []byte {
	key := make([]byte, 0, 2+20+8)
	key = append(key, []byte(transferTxOriginPrefix)...)
	key = append(key, addr[:]...)
	key = append(key, seq.BigEndianBytes()...)
	return key
}

// Prefix keys for range iteration

func eventAddressPrefix(addr thor.Address) []byte {
	prefix := make([]byte, 0, 2+20)
	prefix = append(prefix, []byte(eventAddrPrefix)...)
	prefix = append(prefix, addr[:]...)
	return prefix
}

func eventTopicPrefix(topicIndex int, topic thor.Bytes32) []byte {
	prefixes := []string{eventTopic0Prefix, eventTopic1Prefix, eventTopic2Prefix, eventTopic3Prefix, eventTopic4Prefix}
	if topicIndex < 0 || topicIndex >= len(prefixes) {
		return nil
	}

	prefix := make([]byte, 0, 3+32)
	prefix = append(prefix, []byte(prefixes[topicIndex])...)
	prefix = append(prefix, topic[:]...)
	return prefix
}

func transferSenderPrefixKey(addr thor.Address) []byte {
	prefix := make([]byte, 0, 2+20)
	prefix = append(prefix, []byte(transferSenderPrefix)...)
	prefix = append(prefix, addr[:]...)
	return prefix
}

func transferRecipientPrefixKey(addr thor.Address) []byte {
	prefix := make([]byte, 0, 2+20)
	prefix = append(prefix, []byte(transferRecipientPrefix)...)
	prefix = append(prefix, addr[:]...)
	return prefix
}

func transferTxOriginPrefixKey(addr thor.Address) []byte {
	prefix := make([]byte, 0, 2+20)
	prefix = append(prefix, []byte(transferTxOriginPrefix)...)
	prefix = append(prefix, addr[:]...)
	return prefix
}
