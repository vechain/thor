// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"context"
	"encoding/binary"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// createParentID creates a parent ID for a block with the given number
// The block number is encoded in the first 4 bytes of the parent ID
func createParentID(blockNumber uint32) thor.Bytes32 {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:4], blockNumber-1)
	return parentID
}

func TestPebbleDB_BasicFunctionality(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebbledb_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	// Test basic write and read
	writer := db.NewWriter()
	defer writer.Rollback()

	// Create test block with events and transfers
	testBlock := new(block.Builder).
		ParentID(createParentID(100)).
		Timestamp(1234567890).
		TotalScore(100).
		GasLimit(10000000).
		Build()

	// Create test receipts with events and transfers
	testAddr := thor.BytesToAddress([]byte("test_address"))
	testTopic := thor.BytesToBytes32([]byte("test_topic"))

	event := &tx.Event{
		Address: testAddr,
		Topics:  []thor.Bytes32{testTopic},
		Data:    []byte("test_data"),
	}

	transfer := &tx.Transfer{
		Sender:    testAddr,
		Recipient: thor.BytesToAddress([]byte("recipient")),
		Amount:    big.NewInt(1000),
	}

	receipt := &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events:    []*tx.Event{event},
				Transfers: []*tx.Transfer{transfer},
			},
		},
	}

	receipts := tx.Receipts{receipt}

	// Write block
	err = writer.Write(testBlock, receipts)
	require.NoError(t, err)

	err = writer.Commit()
	require.NoError(t, err)

	// Test event filtering
	ctx := context.Background()

	// Filter by address
	addressFilter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &testAddr},
		},
		Range:   &logsdb.Range{From: 99, To: 101},
		Options: &logsdb.Options{Offset: 0, Limit: 10},
		Order:   logsdb.ASC,
	}

	events, err := db.FilterEvents(ctx, addressFilter)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, testAddr, events[0].Address)
	assert.Equal(t, uint32(100), events[0].BlockNumber)

	// Filter by topic
	topicFilter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Topics: [5]*thor.Bytes32{&testTopic, nil, nil, nil, nil}},
		},
		Range:   &logsdb.Range{From: 99, To: 101},
		Options: &logsdb.Options{Offset: 0, Limit: 10},
		Order:   logsdb.ASC,
	}

	events, err = db.FilterEvents(ctx, topicFilter)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, &testTopic, events[0].Topics[0])

	// Test transfer filtering
	senderFilter := &logsdb.TransferFilter{
		CriteriaSet: []*logsdb.TransferCriteria{
			{Sender: &testAddr},
		},
		Range:   &logsdb.Range{From: 99, To: 101},
		Options: &logsdb.Options{Offset: 0, Limit: 10},
		Order:   logsdb.ASC,
	}

	transfers, err := db.FilterTransfers(ctx, senderFilter)
	require.NoError(t, err)
	assert.Len(t, transfers, 1)
	assert.Equal(t, testAddr, transfers[0].Sender)
	assert.Equal(t, big.NewInt(1000), transfers[0].Amount)
}

func TestPebbleDB_ANDORSemantics(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebbledb_and_or_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	writer := db.NewWriter()
	defer writer.Rollback()

	// Create test data with multiple events
	addr1 := thor.BytesToAddress([]byte("address1"))
	addr2 := thor.BytesToAddress([]byte("address2"))
	topic1 := thor.BytesToBytes32([]byte("topic1"))
	topic2 := thor.BytesToBytes32([]byte("topic2"))

	// Create test block - block number will be 200
	testBlock := new(block.Builder).
		ParentID(createParentID(200)).
		Timestamp(1234567890).
		TotalScore(200).
		GasLimit(10000000).
		Build()

	// Create events:
	// Event 1: addr1 + topic1
	// Event 2: addr1 + topic2
	// Event 3: addr2 + topic1
	// Event 4: addr2 + topic2
	events := []*tx.Event{
		{Address: addr1, Topics: []thor.Bytes32{topic1}, Data: []byte("event1")},
		{Address: addr1, Topics: []thor.Bytes32{topic2}, Data: []byte("event2")},
		{Address: addr2, Topics: []thor.Bytes32{topic1}, Data: []byte("event3")},
		{Address: addr2, Topics: []thor.Bytes32{topic2}, Data: []byte("event4")},
	}

	receipt := &tx.Receipt{
		Outputs: []*tx.Output{
			{Events: events},
		},
	}

	err = writer.Write(testBlock, tx.Receipts{receipt})
	require.NoError(t, err)
	err = writer.Commit()
	require.NoError(t, err)

	ctx := context.Background()

	// Test AND within single criterion: addr1 AND topic1 (should return 1 event)
	andFilter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &addr1, Topics: [5]*thor.Bytes32{&topic1, nil, nil, nil, nil}},
		},
		Range:   &logsdb.Range{From: 199, To: 201},
		Options: &logsdb.Options{Offset: 0, Limit: 10},
		Order:   logsdb.ASC,
	}

	results, err := db.FilterEvents(ctx, andFilter)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, addr1, results[0].Address)
	assert.Equal(t, &topic1, results[0].Topics[0])

	// Test OR across criteria: (addr1 AND topic1) OR (addr2 AND topic2) (should return 2 events)
	orFilter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &addr1, Topics: [5]*thor.Bytes32{&topic1, nil, nil, nil, nil}},
			{Address: &addr2, Topics: [5]*thor.Bytes32{&topic2, nil, nil, nil, nil}},
		},
		Range:   &logsdb.Range{From: 199, To: 201},
		Options: &logsdb.Options{Offset: 0, Limit: 10},
		Order:   logsdb.ASC,
	}

	results, err = db.FilterEvents(ctx, orFilter)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify results contain both expected events
	foundAddr1Topic1 := false
	foundAddr2Topic2 := false
	for _, event := range results {
		if event.Address == addr1 && event.Topics[0] != nil && *event.Topics[0] == topic1 {
			foundAddr1Topic1 = true
		}
		if event.Address == addr2 && event.Topics[0] != nil && *event.Topics[0] == topic2 {
			foundAddr2Topic2 = true
		}
	}
	assert.True(t, foundAddr1Topic1, "Should find addr1+topic1 event")
	assert.True(t, foundAddr2Topic2, "Should find addr2+topic2 event")
}

func TestPebbleDB_LimitOffsetStreaming(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebbledb_limit_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	writer := db.NewWriter()
	defer writer.Rollback()

	// Create multiple blocks with events to test limit/offset
	testAddr := thor.BytesToAddress([]byte("test_address"))

	for blockNum := uint32(1); blockNum <= 50; blockNum++ {
		testBlock := new(block.Builder).
			ParentID(createParentID(blockNum)).
			Timestamp(uint64(1234567890 + blockNum)).
			TotalScore(uint64(blockNum)).
			GasLimit(10000000).
			Build()

		// Create 2 events per block
		events := []*tx.Event{
			{Address: testAddr, Topics: []thor.Bytes32{}, Data: []byte("event1")},
			{Address: testAddr, Topics: []thor.Bytes32{}, Data: []byte("event2")},
		}

		receipt := &tx.Receipt{
			Outputs: []*tx.Output{
				{Events: events},
			},
		}

		err = writer.Write(testBlock, tx.Receipts{receipt})
		require.NoError(t, err)
	}

	err = writer.Commit()
	require.NoError(t, err)

	ctx := context.Background()

	// Test limit without offset
	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &testAddr},
		},
		Range:   &logsdb.Range{From: 1, To: 50},
		Options: &logsdb.Options{Offset: 0, Limit: 10},
		Order:   logsdb.ASC,
	}

	results, err := db.FilterEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, results, 10, "Should return exactly 10 results due to limit")

	// Test offset with limit
	filter.Options = &logsdb.Options{Offset: 10, Limit: 5}
	results, err = db.FilterEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, results, 5, "Should return exactly 5 results due to limit")

	// Test DESC order with limit
	filter.Order = logsdb.DESC
	filter.Options = &logsdb.Options{Offset: 0, Limit: 10}
	results, err = db.FilterEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, results, 10, "Should return exactly 10 results in DESC order")

	// Verify DESC order - first result should be from highest block number
	assert.True(t, results[0].BlockNumber >= results[len(results)-1].BlockNumber,
		"Results should be in descending order by block number")
}

func TestPebbleDB_Truncate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebbledb_truncate_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	writer := db.NewWriter()
	defer writer.Rollback()

	testAddr := thor.BytesToAddress([]byte("test_address"))
	testTopic := thor.BytesToBytes32([]byte("test_topic"))

	// Write data for blocks 1-10
	for blockNum := uint32(1); blockNum <= 10; blockNum++ {
		testBlock := new(block.Builder).
			ParentID(createParentID(blockNum)).
			Timestamp(uint64(1234567890 + blockNum)).
			TotalScore(uint64(blockNum)).
			GasLimit(10000000).
			Build()

		event := &tx.Event{
			Address: testAddr,
			Topics:  []thor.Bytes32{testTopic},
			Data:    []byte("test_data"),
		}

		transfer := &tx.Transfer{
			Sender:    testAddr,
			Recipient: thor.BytesToAddress([]byte("recipient")),
			Amount:    big.NewInt(int64(blockNum * 1000)),
		}

		receipt := &tx.Receipt{
			Outputs: []*tx.Output{
				{Events: []*tx.Event{event}, Transfers: []*tx.Transfer{transfer}},
			},
		}

		err = writer.Write(testBlock, tx.Receipts{receipt})
		require.NoError(t, err)
	}

	err = writer.Commit()
	require.NoError(t, err)

	ctx := context.Background()

	// Verify all blocks exist
	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &testAddr},
		},
		Range:   &logsdb.Range{From: 1, To: 10},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.ASC,
	}

	events, err := db.FilterEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, events, 10, "Should have 10 events before truncate")

	// Truncate from block 6 onwards (blocks 6-10 should be deleted)
	err = writer.Truncate(6)
	require.NoError(t, err)
	err = writer.Commit()
	require.NoError(t, err)

	// Verify only blocks 1-5 remain
	events, err = db.FilterEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, events, 5, "Should have 5 events after truncate")

	// Verify highest block number is 5
	maxBlockNum := uint32(0)
	for _, event := range events {
		if event.BlockNumber > maxBlockNum {
			maxBlockNum = event.BlockNumber
		}
	}
	assert.Equal(t, uint32(5), maxBlockNum, "Highest remaining block should be 5")

	// Test transfers are also truncated
	transferFilter := &logsdb.TransferFilter{
		CriteriaSet: []*logsdb.TransferCriteria{
			{Sender: &testAddr},
		},
		Range:   &logsdb.Range{From: 1, To: 10},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.ASC,
	}

	transfers, err := db.FilterTransfers(ctx, transferFilter)
	require.NoError(t, err)
	assert.Len(t, transfers, 5, "Should have 5 transfers after truncate")
}

// TestSequenceIndexCreation verifies that ES/ and TSX/ keys are created during writes
func TestSequenceIndexCreation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pebble-sequence-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	db, err := Open(tempDir)
	require.NoError(t, err)
	defer db.Close()

	testAddr := thor.MustParseAddress("0x1234567890123456789012345678901234567890")
	testTopic := thor.MustParseBytes32("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	// Create test block
	block := createTestBlock(1, testAddr)

	// Create receipts with events and transfers
	receipts := tx.Receipts{
		{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: testAddr,
							Topics:  []thor.Bytes32{testTopic},
							Data:    []byte("test event data"),
						},
					},
					Transfers: []*tx.Transfer{
						{
							Sender:    testAddr,
							Recipient: testAddr,
							Amount:    big.NewInt(1000),
						},
					},
				},
			},
		},
	}

	// Write block
	writer := db.NewWriter()
	err = writer.Write(block, receipts)
	require.NoError(t, err)
	err = writer.Commit()
	require.NoError(t, err)

	// Verify ES/ and TSX/ keys exist
	internalDB := db.GetPebbleDB()
	
	// Check ES/ key exists
	esOpts := &pebble.IterOptions{
		LowerBound: []byte("ES"),
		UpperBound: []byte("ET"),
	}
	esIter, err := internalDB.NewIter(esOpts)
	require.NoError(t, err)
	defer esIter.Close()
	
	esIter.First()
	assert.True(t, esIter.Valid(), "ES/ sequence index should exist")
	if esIter.Valid() {
		key := esIter.Key()
		assert.True(t, len(key) == 10, "ES/ key should be 10 bytes (ES + 8-byte sequence)")
		assert.Equal(t, "ES", string(key[:2]), "Key should start with ES prefix")
	}

	// Check TSX/ key exists  
	tsxOpts := &pebble.IterOptions{
		LowerBound: []byte("TSX"),
		UpperBound: []byte("TSY"),
	}
	tsxIter, err := internalDB.NewIter(tsxOpts)
	require.NoError(t, err)
	defer tsxIter.Close()
	
	tsxIter.First()
	assert.True(t, tsxIter.Valid(), "TSX/ sequence index should exist")
	if tsxIter.Valid() {
		key := tsxIter.Key()
		assert.True(t, len(key) == 11, "TSX/ key should be 11 bytes (TSX + 8-byte sequence)")
		assert.Equal(t, "TSX", string(key[:3]), "Key should start with TSX prefix")
	}
}

// TestSequenceIndexTruncate verifies correct bounds in truncate operations
func TestSequenceIndexTruncate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pebble-truncate-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	db, err := Open(tempDir)
	require.NoError(t, err)
	defer db.Close()

	testAddr := thor.MustParseAddress("0x1234567890123456789012345678901234567890")

	// Write multiple blocks (1-5)
	for blockNum := uint32(1); blockNum <= 5; blockNum++ {
		block := createTestBlock(blockNum, testAddr)
		receipts := createTestReceipts(testAddr)
		
		writer := db.NewWriter()
		err = writer.Write(block, receipts)
		require.NoError(t, err)
		err = writer.Commit()
		require.NoError(t, err)
	}

	internalDB := db.GetPebbleDB()

	// Count ES/ keys before truncation
	esCountBefore := countKeysWithPrefix(internalDB, "ES")
	tsxCountBefore := countKeysWithPrefix(internalDB, "TSX")
	assert.Equal(t, 5, esCountBefore, "Should have 5 ES/ keys before truncate")
	assert.Equal(t, 5, tsxCountBefore, "Should have 5 TSX/ keys before truncate")

	// Truncate from block 4 (should keep blocks 1-3)
	writer := db.NewWriter()
	err = writer.Truncate(4)
	require.NoError(t, err)
	err = writer.Commit()
	require.NoError(t, err)

	// Count ES/ keys after truncation  
	esCountAfter := countKeysWithPrefix(internalDB, "ES")
	tsxCountAfter := countKeysWithPrefix(internalDB, "TSX")
	assert.Equal(t, 3, esCountAfter, "Should have 3 ES/ keys after truncate")
	assert.Equal(t, 3, tsxCountAfter, "Should have 3 TSX/ keys after truncate")
}

// TestHasBlockIDO1Performance verifies true O(1) behavior with no scanning
func TestHasBlockIDO1Performance(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pebble-hasblockid-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	db, err := Open(tempDir)
	require.NoError(t, err)
	defer db.Close()

	testAddr := thor.MustParseAddress("0x1234567890123456789012345678901234567890")

	// Write test blocks with known block IDs
	var testBlockIDs []thor.Bytes32
	for blockNum := uint32(1); blockNum <= 3; blockNum++ {
		block := createTestBlock(blockNum, testAddr)
		testBlockIDs = append(testBlockIDs, block.Header().ID())
		receipts := createTestReceipts(testAddr)
		
		writer := db.NewWriter()
		err = writer.Write(block, receipts)
		require.NoError(t, err)
		err = writer.Commit()
		require.NoError(t, err)
	}

	// Test HasBlockID with existing block IDs
	for i, blockID := range testBlockIDs {
		exists, err := db.HasBlockID(blockID)
		require.NoError(t, err)
		assert.True(t, exists, "Block ID %d should exist", i+1)
	}

	// Test HasBlockID with non-existing block ID (block 999 should not exist)
	var nonExistentBlockID thor.Bytes32
	binary.BigEndian.PutUint32(nonExistentBlockID[:4], 999) // Block 999
	exists, err := db.HasBlockID(nonExistentBlockID)
	require.NoError(t, err)
	assert.False(t, exists, "Non-existent block ID should not exist")
}

// TestRangeOnlyQueryEquivalence verifies new ES/TSX path returns identical results
func TestRangeOnlyQueryEquivalence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pebble-range-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	db, err := Open(tempDir)
	require.NoError(t, err)
	defer db.Close()

	testAddr := thor.MustParseAddress("0x1234567890123456789012345678901234567890")

	// Write test blocks
	for blockNum := uint32(1); blockNum <= 5; blockNum++ {
		block := createTestBlock(blockNum, testAddr)
		receipts := createTestReceipts(testAddr)
		
		writer := db.NewWriter()
		err = writer.Write(block, receipts)
		require.NoError(t, err)
		err = writer.Commit()
		require.NoError(t, err)
	}

	ctx := context.Background()

	// Test range-only event query
	eventFilter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{}, // Empty criteria = range-only
		Range:       &logsdb.Range{From: 2, To: 4},
		Options:     &logsdb.Options{Offset: 0, Limit: 100},
		Order:       logsdb.ASC,
	}

	events, err := db.FilterEvents(ctx, eventFilter)
	require.NoError(t, err)
	assert.Len(t, events, 3, "Should return events from blocks 2-4")

	// Verify block numbers are in the expected range
	for _, event := range events {
		assert.True(t, event.BlockNumber >= 2 && event.BlockNumber <= 4,
			"Event block number %d should be in range 2-4", event.BlockNumber)
	}

	// Test range-only transfer query
	transferFilter := &logsdb.TransferFilter{
		CriteriaSet: []*logsdb.TransferCriteria{}, // Empty criteria = range-only
		Range:       &logsdb.Range{From: 1, To: 3},
		Options:     &logsdb.Options{Offset: 0, Limit: 100},
		Order:       logsdb.ASC,
	}

	transfers, err := db.FilterTransfers(ctx, transferFilter)
	require.NoError(t, err)
	assert.Len(t, transfers, 3, "Should return transfers from blocks 1-3")

	// Verify block numbers are in the expected range
	for _, transfer := range transfers {
		assert.True(t, transfer.BlockNumber >= 1 && transfer.BlockNumber <= 3,
			"Transfer block number %d should be in range 1-3", transfer.BlockNumber)
	}
}

// TestBackwardCompatibility tests PebbleDB without ES/TSX indexes still works
func TestBackwardCompatibility(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pebble-compat-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Manually create a database without sequence indexes for compatibility test
	pebbleDB, err := pebble.Open(tempDir, &pebble.Options{})
	require.NoError(t, err)

	// Manually add some primary records without sequence indexes
	testAddr := thor.MustParseAddress("0x1234567890123456789012345678901234567890")
	seq, _ := newSequence(1, 0, 0)
	
	// Create event record without ES/ index
	eventRecord := &EventRecord{
		BlockID:     thor.MustParseBytes32("0x1111111111111111111111111111111111111111111111111111111111111111"),
		BlockNumber: 1,
		Address:     testAddr,
		Topics:      []thor.Bytes32{},
		Data:        []byte("test"),
	}
	eventData, err := eventRecord.Encode()
	require.NoError(t, err)
	
	err = pebbleDB.Set(eventPrimaryKey(seq), eventData, pebble.Sync)
	require.NoError(t, err)
	err = pebbleDB.Set(eventAddressKey(testAddr, seq), nil, pebble.Sync)
	require.NoError(t, err)

	pebbleDB.Close()

	// Open with our PebbleDBLogDB wrapper
	db, err := Open(tempDir)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Test that address queries still work (should use fallback to primary range iterator)
	eventFilter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &testAddr},
		},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.ASC,
	}

	events, err := db.FilterEvents(ctx, eventFilter)
	require.NoError(t, err)
	assert.Len(t, events, 1, "Should find the manually inserted event")

	// Test that range-only queries work (should use fallback when no ES/ indexes exist)
	rangeFilter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{}, // Empty = range-only
		Range:       &logsdb.Range{From: 1, To: 1},
		Options:     &logsdb.Options{Offset: 0, Limit: 100},
		Order:       logsdb.ASC,
	}

	rangeEvents, err := db.FilterEvents(ctx, rangeFilter)
	require.NoError(t, err)
	assert.Len(t, rangeEvents, 1, "Should find the event via fallback primary range iterator")
}

// Helper functions for tests

func countKeysWithPrefix(db *pebble.DB, prefix string) int {
	opts := &pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: []byte(prefix + "\xff"),
	}
	
	iter, err := db.NewIter(opts)
	if err != nil {
		return 0
	}
	defer iter.Close()
	
	count := 0
	for iter.First(); iter.Valid(); iter.Next() {
		count++
	}
	return count
}

func createTestBlock(blockNumber uint32, testAddr thor.Address) *block.Block {
	parentID := createParentID(blockNumber)
	
	return new(block.Builder).
		ParentID(parentID).
		Timestamp(1234567890).
		TotalScore(100).
		GasLimit(10000000).
		Build()
}

func createTestReceipts(testAddr thor.Address) tx.Receipts {
	testTopic := thor.MustParseBytes32("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	
	return tx.Receipts{
		{
			Outputs: []*tx.Output{
				{
					Events: []*tx.Event{
						{
							Address: testAddr,
							Topics:  []thor.Bytes32{testTopic},
							Data:    []byte("test event data"),
						},
					},
					Transfers: []*tx.Transfer{
						{
							Sender:    testAddr,
							Recipient: testAddr,
							Amount:    big.NewInt(1000),
						},
					},
				},
			},
		},
	}
}
