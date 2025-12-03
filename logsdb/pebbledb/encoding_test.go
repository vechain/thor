// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
)

func TestEventRecord_RoundTrip_Binary(t *testing.T) {
	tests := []struct {
		name   string
		record *EventRecord
	}{
		{
			name: "empty event",
			record: &EventRecord{
				BlockID:     thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
				BlockNumber: 123,
				BlockTime:   1640995200,
				TxID:        thor.MustParseBytes32("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
				TxIndex:     5,
				TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
				ClauseIndex: 2,
				LogIndex:    10,
				Address:     thor.MustParseAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
				Topics:      nil,
				Data:        nil,
			},
		},
		{
			name: "event with topics only",
			record: &EventRecord{
				BlockID:     thor.MustParseBytes32("0x1111111111111111111111111111111111111111111111111111111111111111"),
				BlockNumber: 456,
				BlockTime:   1640995300,
				TxID:        thor.MustParseBytes32("0x2222222222222222222222222222222222222222222222222222222222222222"),
				TxIndex:     3,
				TxOrigin:    thor.MustParseAddress("0x3333333333333333333333333333333333333333"),
				ClauseIndex: 1,
				LogIndex:    20,
				Address:     thor.MustParseAddress("0x4444444444444444444444444444444444444444"),
				Topics: []thor.Bytes32{
					thor.MustParseBytes32("0x5555555555555555555555555555555555555555555555555555555555555555"),
					thor.MustParseBytes32("0x6666666666666666666666666666666666666666666666666666666666666666"),
				},
				Data: nil,
			},
		},
		{
			name: "event with data only",
			record: &EventRecord{
				BlockID:     thor.MustParseBytes32("0x7777777777777777777777777777777777777777777777777777777777777777"),
				BlockNumber: 789,
				BlockTime:   1640995400,
				TxID:        thor.MustParseBytes32("0x8888888888888888888888888888888888888888888888888888888888888888"),
				TxIndex:     7,
				TxOrigin:    thor.MustParseAddress("0x9999999999999999999999999999999999999999"),
				ClauseIndex: 4,
				LogIndex:    30,
				Address:     thor.MustParseAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				Topics:      nil,
				Data:        []byte("test data for event"),
			},
		},
		{
			name: "event with max topics and data",
			record: &EventRecord{
				BlockID:     thor.MustParseBytes32("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
				BlockNumber: 999,
				BlockTime:   1640995500,
				TxID:        thor.MustParseBytes32("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
				TxIndex:     65535, // max uint16
				TxOrigin:    thor.MustParseAddress("0xdddddddddddddddddddddddddddddddddddddddd"),
				ClauseIndex: 65535, // max uint16
				LogIndex:    4294967295, // max uint32
				Address:     thor.MustParseAddress("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
				Topics: []thor.Bytes32{
					thor.MustParseBytes32("0xf000000000000000000000000000000000000000000000000000000000000000"),
					thor.MustParseBytes32("0xf111111111111111111111111111111111111111111111111111111111111111"),
					thor.MustParseBytes32("0xf222222222222222222222222222222222222222222222222222222222222222"),
					thor.MustParseBytes32("0xf333333333333333333333333333333333333333333333333333333333333333"),
					thor.MustParseBytes32("0xf444444444444444444444444444444444444444444444444444444444444444"),
				},
				Data: []byte("maximum test data with topics and all fields populated to test edge cases"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := EncodeEventRecord(tt.record)
			require.NoError(t, err)
			require.NotEmpty(t, encoded)

			// Verify binary encoding header
			require.True(t, len(encoded) >= 5, "Binary encoding should have header")
			assert.True(t, encoded[0]&FlagIsEvent != 0, "Should have event flag set")

			// Decode
			decoded, err := DecodeEventRecord(encoded)
			require.NoError(t, err)
			require.NotNil(t, decoded)

			// Compare all fields
			assert.Equal(t, tt.record.BlockID, decoded.BlockID)
			assert.Equal(t, tt.record.BlockNumber, decoded.BlockNumber)
			assert.Equal(t, tt.record.BlockTime, decoded.BlockTime)
			assert.Equal(t, tt.record.TxID, decoded.TxID)
			assert.Equal(t, tt.record.TxIndex, decoded.TxIndex)
			assert.Equal(t, tt.record.TxOrigin, decoded.TxOrigin)
			assert.Equal(t, tt.record.ClauseIndex, decoded.ClauseIndex)
			assert.Equal(t, tt.record.LogIndex, decoded.LogIndex)
			assert.Equal(t, tt.record.Address, decoded.Address)
			assert.Equal(t, tt.record.Topics, decoded.Topics)
			assert.Equal(t, tt.record.Data, decoded.Data)

			// Deep equality check
			assert.True(t, reflect.DeepEqual(tt.record, decoded))
		})
	}
}

func TestTransferRecord_RoundTrip_Binary(t *testing.T) {
	tests := []struct {
		name   string
		record *TransferRecord
	}{
		{
			name: "basic transfer",
			record: &TransferRecord{
				BlockID:     thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
				BlockNumber: 123,
				BlockTime:   1640995200,
				TxID:        thor.MustParseBytes32("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
				TxIndex:     5,
				TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
				ClauseIndex: 2,
				LogIndex:    10,
				Sender:      thor.MustParseAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				Recipient:   thor.MustParseAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
				Amount:      big.NewInt(1000000000000000000), // 1 VET
			},
		},
		{
			name: "max values transfer",
			record: &TransferRecord{
				BlockID:     thor.MustParseBytes32("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
				BlockNumber: 4294967295, // max uint32
				BlockTime:   18446744073709551615, // max uint64
				TxID:        thor.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000000000"),
				TxIndex:     65535, // max uint16
				TxOrigin:    thor.MustParseAddress("0x0000000000000000000000000000000000000000"),
				ClauseIndex: 65535, // max uint16
				LogIndex:    4294967295, // max uint32
				Sender:      thor.MustParseAddress("0xffffffffffffffffffffffffffffffffffffffff"),
				Recipient:   thor.MustParseAddress("0x1111111111111111111111111111111111111111"),
				Amount: func() *big.Int {
					// Max uint256
					max := new(big.Int)
					max.SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10)
					return max
				}(),
			},
		},
		{
			name: "zero transfer",
			record: &TransferRecord{
				BlockID:     thor.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000000000"),
				BlockNumber: 0,
				BlockTime:   0,
				TxID:        thor.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000000000"),
				TxIndex:     0,
				TxOrigin:    thor.MustParseAddress("0x0000000000000000000000000000000000000000"),
				ClauseIndex: 0,
				LogIndex:    0,
				Sender:      thor.MustParseAddress("0x0000000000000000000000000000000000000000"),
				Recipient:   thor.MustParseAddress("0x0000000000000000000000000000000000000000"),
				Amount:      big.NewInt(0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := EncodeTransferRecord(tt.record)
			require.NoError(t, err)
			require.NotEmpty(t, encoded)

			// Verify binary encoding header
			require.True(t, len(encoded) >= 5, "Binary encoding should have header")
			assert.True(t, encoded[0]&FlagIsEvent == 0, "Should not have event flag set for transfers")

			// Decode
			decoded, err := DecodeTransferRecord(encoded)
			require.NoError(t, err)
			require.NotNil(t, decoded)

			// Compare all fields
			assert.Equal(t, tt.record.BlockID, decoded.BlockID)
			assert.Equal(t, tt.record.BlockNumber, decoded.BlockNumber)
			assert.Equal(t, tt.record.BlockTime, decoded.BlockTime)
			assert.Equal(t, tt.record.TxID, decoded.TxID)
			assert.Equal(t, tt.record.TxIndex, decoded.TxIndex)
			assert.Equal(t, tt.record.TxOrigin, decoded.TxOrigin)
			assert.Equal(t, tt.record.ClauseIndex, decoded.ClauseIndex)
			assert.Equal(t, tt.record.LogIndex, decoded.LogIndex)
			assert.Equal(t, tt.record.Sender, decoded.Sender)
			assert.Equal(t, tt.record.Recipient, decoded.Recipient)
			assert.Equal(t, 0, tt.record.Amount.Cmp(decoded.Amount))

			// Field-by-field comparison (big.Int doesn't work well with reflect.DeepEqual)
			assert.Equal(t, tt.record.BlockID, decoded.BlockID)
			assert.Equal(t, tt.record.BlockNumber, decoded.BlockNumber)
			assert.Equal(t, tt.record.BlockTime, decoded.BlockTime)
			assert.Equal(t, tt.record.TxID, decoded.TxID)
			assert.Equal(t, tt.record.TxIndex, decoded.TxIndex)
			assert.Equal(t, tt.record.TxOrigin, decoded.TxOrigin)
			assert.Equal(t, tt.record.ClauseIndex, decoded.ClauseIndex)
			assert.Equal(t, tt.record.LogIndex, decoded.LogIndex)
			assert.Equal(t, tt.record.Sender, decoded.Sender)
			assert.Equal(t, tt.record.Recipient, decoded.Recipient)
			assert.Equal(t, 0, tt.record.Amount.Cmp(decoded.Amount))
		})
	}
}

func TestEncoding_ErrorCases(t *testing.T) {
	t.Run("nil event record", func(t *testing.T) {
		_, err := EncodeEventRecord(nil)
		assert.Error(t, err)
	})

	t.Run("nil transfer record", func(t *testing.T) {
		_, err := EncodeTransferRecord(nil)
		assert.Error(t, err)
	})

	t.Run("corrupted binary data", func(t *testing.T) {
		// Too short
		_, err := DecodeEventRecord([]byte{1})
		assert.Error(t, err)

		// Header only
		_, err = DecodeEventRecord([]byte{1, 1, 0, 0, 0, 0})
		assert.Error(t, err)
	})

	t.Run("invalid topics count", func(t *testing.T) {
		// Create a valid header and fixed section, then add invalid topics count
		corrupted := []byte{
			FlagIsEvent | FlagHasTopics, // flags: event with topics
		}
		// Add valid field mask
		fieldMask := EventRequiredFields
		corrupted = append(corrupted, byte(fieldMask>>24), byte(fieldMask>>16), byte(fieldMask>>8), byte(fieldMask))
		
		// Add valid fixed fields (128 bytes for common + address fields)
		corrupted = append(corrupted, make([]byte, 128)...)
		
		// Add invalid topics count > 5
		corrupted = append(corrupted, 6) // Invalid topics count
		
		_, err := DecodeEventRecord(corrupted)
		assert.Error(t, err)
		// This should trigger the invalid topics count error
		if !assert.Contains(t, err.Error(), "invalid topics count") {
			t.Logf("Got error: %v", err)
		}
	})

	t.Run("wrong record type detection", func(t *testing.T) {
		// Encode an event
		eventRecord := &EventRecord{
			BlockID:     thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
			BlockNumber: 123,
			BlockTime:   1640995200,
			TxID:        thor.MustParseBytes32("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
			TxIndex:     5,
			TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
			ClauseIndex: 2,
			LogIndex:    10,
			Address:     thor.MustParseAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		}

		encoded, err := EncodeEventRecord(eventRecord)
		require.NoError(t, err)

		// Try to decode as transfer (should fail)
		_, err = DecodeTransferRecord(encoded)
		assert.Error(t, err)
	})
}

func TestFieldMaskValidation(t *testing.T) {
	tests := []struct {
		name      string
		fieldMask uint32
		isEvent   bool
		wantError bool
	}{
		{
			name:      "valid event mask",
			fieldMask: EventRequiredFields,
			isEvent:   true,
			wantError: false,
		},
		{
			name:      "valid transfer mask",
			fieldMask: TransferRequiredFields,
			isEvent:   false,
			wantError: false,
		},
		{
			name:      "missing event field",
			fieldMask: EventRequiredFields &^ FieldAddress,
			isEvent:   true,
			wantError: true,
		},
		{
			name:      "missing transfer field",
			fieldMask: TransferRequiredFields &^ FieldAmount,
			isEvent:   false,
			wantError: true,
		},
		{
			name:      "future field bits (should be ignored)",
			fieldMask: EventRequiredFields | (1 << 20),
			isEvent:   true,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFieldMask(tt.fieldMask, tt.isEvent)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Benchmarks
func BenchmarkEncodeEventRecord_Binary(b *testing.B) {
	record := &EventRecord{
		BlockID:     thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
		BlockNumber: 123456,
		BlockTime:   1640995200,
		TxID:        thor.MustParseBytes32("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		TxIndex:     5,
		TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
		ClauseIndex: 2,
		LogIndex:    10,
		Address:     thor.MustParseAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		Topics: []thor.Bytes32{
			thor.MustParseBytes32("0x5555555555555555555555555555555555555555555555555555555555555555"),
			thor.MustParseBytes32("0x6666666666666666666666666666666666666666666666666666666666666666"),
		},
		Data: []byte("benchmark test data for encoding performance"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := EncodeEventRecord(record)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeEventRecord_Binary(b *testing.B) {
	record := &EventRecord{
		BlockID:     thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
		BlockNumber: 123456,
		BlockTime:   1640995200,
		TxID:        thor.MustParseBytes32("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		TxIndex:     5,
		TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
		ClauseIndex: 2,
		LogIndex:    10,
		Address:     thor.MustParseAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		Topics: []thor.Bytes32{
			thor.MustParseBytes32("0x5555555555555555555555555555555555555555555555555555555555555555"),
			thor.MustParseBytes32("0x6666666666666666666666666666666666666666666666666666666666666666"),
		},
		Data: []byte("benchmark test data for encoding performance"),
	}

	encoded, err := EncodeEventRecord(record)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DecodeEventRecord(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeTransferRecord_Binary(b *testing.B) {
	record := &TransferRecord{
		BlockID:     thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
		BlockNumber: 123456,
		BlockTime:   1640995200,
		TxID:        thor.MustParseBytes32("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		TxIndex:     5,
		TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
		ClauseIndex: 2,
		LogIndex:    10,
		Sender:      thor.MustParseAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Recipient:   thor.MustParseAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Amount:      big.NewInt(1000000000000000000),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := EncodeTransferRecord(record)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestBinaryEncodingSize(t *testing.T) {
	// Create test records
	eventRecord := &EventRecord{
		BlockID:     thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
		BlockNumber: 123456,
		BlockTime:   1640995200,
		TxID:        thor.MustParseBytes32("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		TxIndex:     5,
		TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
		ClauseIndex: 2,
		LogIndex:    10,
		Address:     thor.MustParseAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		Topics: []thor.Bytes32{
			thor.MustParseBytes32("0x5555555555555555555555555555555555555555555555555555555555555555"),
			thor.MustParseBytes32("0x6666666666666666666666666666666666666666666666666666666666666666"),
		},
		Data: []byte("test data for size comparison"),
	}

	transferRecord := &TransferRecord{
		BlockID:     thor.MustParseBytes32("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
		BlockNumber: 123456,
		BlockTime:   1640995200,
		TxID:        thor.MustParseBytes32("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		TxIndex:     5,
		TxOrigin:    thor.MustParseAddress("0x1234567890123456789012345678901234567890"),
		ClauseIndex: 2,
		LogIndex:    10,
		Sender:      thor.MustParseAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Recipient:   thor.MustParseAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Amount:      big.NewInt(1000000000000000000),
	}

	// Encode with binary
	eventBinary, err := EncodeEventRecord(eventRecord)
	require.NoError(t, err)
	transferBinary, err := EncodeTransferRecord(transferRecord)
	require.NoError(t, err)

	t.Logf("Event - Binary: %d bytes", len(eventBinary))
	t.Logf("Transfer - Binary: %d bytes", len(transferBinary))

	// Verify reasonable sizes for binary encoding
	assert.Greater(t, len(eventBinary), 100, "Event binary should have reasonable minimum size")
	assert.Greater(t, len(transferBinary), 150, "Transfer binary should have reasonable minimum size")
}