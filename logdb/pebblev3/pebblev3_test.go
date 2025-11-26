// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebblev3

import (
	"context"
	"encoding/binary"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logdb"
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

func TestPebbleV3_BasicFunctionality(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebblev3_test")
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
	addressFilter := &logdb.EventFilter{
		CriteriaSet: []*logdb.EventCriteria{
			{Address: &testAddr},
		},
		Range:   &logdb.Range{From: 99, To: 101},
		Options: &logdb.Options{Offset: 0, Limit: 10},
		Order:   logdb.ASC,
	}

	events, err := db.FilterEvents(ctx, addressFilter)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, testAddr, events[0].Address)
	assert.Equal(t, uint32(100), events[0].BlockNumber)

	// Filter by topic
	topicFilter := &logdb.EventFilter{
		CriteriaSet: []*logdb.EventCriteria{
			{Topics: [5]*thor.Bytes32{&testTopic, nil, nil, nil, nil}},
		},
		Range:   &logdb.Range{From: 99, To: 101},
		Options: &logdb.Options{Offset: 0, Limit: 10},
		Order:   logdb.ASC,
	}

	events, err = db.FilterEvents(ctx, topicFilter)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, &testTopic, events[0].Topics[0])

	// Test transfer filtering
	senderFilter := &logdb.TransferFilter{
		CriteriaSet: []*logdb.TransferCriteria{
			{Sender: &testAddr},
		},
		Range:   &logdb.Range{From: 99, To: 101},
		Options: &logdb.Options{Offset: 0, Limit: 10},
		Order:   logdb.ASC,
	}

	transfers, err := db.FilterTransfers(ctx, senderFilter)
	require.NoError(t, err)
	assert.Len(t, transfers, 1)
	assert.Equal(t, testAddr, transfers[0].Sender)
	assert.Equal(t, big.NewInt(1000), transfers[0].Amount)
}

func TestPebbleV3_ANDORSemantics(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebblev3_and_or_test")
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
	andFilter := &logdb.EventFilter{
		CriteriaSet: []*logdb.EventCriteria{
			{Address: &addr1, Topics: [5]*thor.Bytes32{&topic1, nil, nil, nil, nil}},
		},
		Range:   &logdb.Range{From: 199, To: 201},
		Options: &logdb.Options{Offset: 0, Limit: 10},
		Order:   logdb.ASC,
	}

	results, err := db.FilterEvents(ctx, andFilter)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, addr1, results[0].Address)
	assert.Equal(t, &topic1, results[0].Topics[0])


	// Test OR across criteria: (addr1 AND topic1) OR (addr2 AND topic2) (should return 2 events)
	orFilter := &logdb.EventFilter{
		CriteriaSet: []*logdb.EventCriteria{
			{Address: &addr1, Topics: [5]*thor.Bytes32{&topic1, nil, nil, nil, nil}},
			{Address: &addr2, Topics: [5]*thor.Bytes32{&topic2, nil, nil, nil, nil}},
		},
		Range:   &logdb.Range{From: 199, To: 201},
		Options: &logdb.Options{Offset: 0, Limit: 10},
		Order:   logdb.ASC,
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

func TestPebbleV3_LimitOffsetStreaming(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebblev3_limit_test")
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
	filter := &logdb.EventFilter{
		CriteriaSet: []*logdb.EventCriteria{
			{Address: &testAddr},
		},
		Range:   &logdb.Range{From: 1, To: 50},
		Options: &logdb.Options{Offset: 0, Limit: 10},
		Order:   logdb.ASC,
	}

	results, err := db.FilterEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, results, 10, "Should return exactly 10 results due to limit")

	// Test offset with limit
	filter.Options = &logdb.Options{Offset: 10, Limit: 5}
	results, err = db.FilterEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, results, 5, "Should return exactly 5 results due to limit")

	// Test DESC order with limit
	filter.Order = logdb.DESC
	filter.Options = &logdb.Options{Offset: 0, Limit: 10}
	results, err = db.FilterEvents(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, results, 10, "Should return exactly 10 results in DESC order")

	// Verify DESC order - first result should be from highest block number
	assert.True(t, results[0].BlockNumber >= results[len(results)-1].BlockNumber,
		"Results should be in descending order by block number")
}

func TestPebbleV3_Truncate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebblev3_truncate_test")
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
	filter := &logdb.EventFilter{
		CriteriaSet: []*logdb.EventCriteria{
			{Address: &testAddr},
		},
		Range:   &logdb.Range{From: 1, To: 10},
		Options: &logdb.Options{Offset: 0, Limit: 100},
		Order:   logdb.ASC,
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
	transferFilter := &logdb.TransferFilter{
		CriteriaSet: []*logdb.TransferCriteria{
			{Sender: &testAddr},
		},
		Range:   &logdb.Range{From: 1, To: 10},
		Options: &logdb.Options{Offset: 0, Limit: 100},
		Order:   logdb.ASC,
	}

	transfers, err := db.FilterTransfers(ctx, transferFilter)
	require.NoError(t, err)
	assert.Len(t, transfers, 5, "Should have 5 transfers after truncate")
}