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
	"runtime"
	"testing"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// createParentIDForBench creates a parent ID for a block with the given number
func createParentIDForBench(blockNumber int) thor.Bytes32 {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:4], uint32(blockNumber-1))
	return parentID
}

const (
	BENCH_VTHO_ADDRESS = "0x0000000000000000000000000000456E65726779"
	BENCH_VTHO_TOPIC   = "0xDDF252AD1BE2C89B69C2B068FC378DAA952BA7F163C4A11628F55A4DF523B3EF"
	BENCH_TEST_ADDRESS = "0x7567D83B7B8D80ADDCB281A71D54FC7B3364FFED"
)

func BenchmarkPebbleDB_FilterEvents_SingleAddress(b *testing.B) {
	db, cleanup := setupBenchmarkDB(b, 1000) // 1000 blocks
	defer cleanup()

	vthoAddress := thor.MustParseAddress(BENCH_VTHO_ADDRESS)
	ctx := context.Background()

	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &vthoAddress},
		},
		Range:   &logsdb.Range{From: 1, To: 1000},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}
	}
}

func BenchmarkPebbleDB_FilterEvents_SingleTopic(b *testing.B) {
	db, cleanup := setupBenchmarkDB(b, 1000)
	defer cleanup()

	topic := thor.MustParseBytes32(BENCH_VTHO_TOPIC)
	ctx := context.Background()

	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil}},
		},
		Range:   &logsdb.Range{From: 1, To: 1000},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}
	}
}

func BenchmarkPebbleDB_FilterEvents_AddressAndTopic(b *testing.B) {
	db, cleanup := setupBenchmarkDB(b, 1000)
	defer cleanup()

	vthoAddress := thor.MustParseAddress(BENCH_VTHO_ADDRESS)
	topic := thor.MustParseBytes32(BENCH_VTHO_TOPIC)
	ctx := context.Background()

	// AND within single criterion
	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &vthoAddress, Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil}},
		},
		Range:   &logsdb.Range{From: 1, To: 1000},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}
	}
}

func BenchmarkPebbleDB_FilterEvents_MultiCriteriaOR(b *testing.B) {
	db, cleanup := setupBenchmarkDB(b, 1000)
	defer cleanup()

	vthoAddress := thor.MustParseAddress(BENCH_VTHO_ADDRESS)
	testAddress := thor.MustParseAddress(BENCH_TEST_ADDRESS)
	topic := thor.MustParseBytes32(BENCH_VTHO_TOPIC)
	ctx := context.Background()

	// OR across multiple criteria
	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &vthoAddress},
			{Address: &testAddress},
			{Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil}},
		},
		Range:   &logsdb.Range{From: 1, To: 1000},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}
	}
}

func BenchmarkPebbleDB_FilterEvents_DESC_Order(b *testing.B) {
	db, cleanup := setupBenchmarkDB(b, 1000)
	defer cleanup()

	vthoAddress := thor.MustParseAddress(BENCH_VTHO_ADDRESS)
	ctx := context.Background()

	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &vthoAddress},
		},
		Range:   &logsdb.Range{From: 1, To: 1000},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.DESC, // Test DESC order performance
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}
	}
}

func BenchmarkPebbleDB_FilterTransfers(b *testing.B) {
	db, cleanup := setupBenchmarkDB(b, 1000)
	defer cleanup()

	testAddress := thor.MustParseAddress(BENCH_TEST_ADDRESS)
	ctx := context.Background()

	filter := &logsdb.TransferFilter{
		CriteriaSet: []*logsdb.TransferCriteria{
			{Sender: &testAddress},
		},
		Range:   &logsdb.Range{From: 1, To: 1000},
		Options: &logsdb.Options{Offset: 0, Limit: 100},
		Order:   logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterTransfers(ctx, filter)
		if err != nil {
			b.Fatalf("FilterTransfers failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}
	}
}

func BenchmarkPebbleDB_Write(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "pebbledb_bench_write")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := Open(filepath.Join(tmpDir, "bench.db"))
	if err != nil {
		b.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Prepare test blocks
	blocks := make([]*block.Block, b.N)
	receipts := make([]tx.Receipts, b.N)

	for i := 0; b.Loop(); i++ {
		blocks[i] = new(block.Builder).
			ParentID(createParentIDForBench(i + 1)).
			Timestamp(uint64(1234567890 + i)).
			TotalScore(uint64(i + 1)).
			GasLimit(10000000).
			Build()

		// Create test events and transfers
		vthoAddr := thor.MustParseAddress(BENCH_VTHO_ADDRESS)
		testAddr := thor.MustParseAddress(BENCH_TEST_ADDRESS)
		topic := thor.MustParseBytes32(BENCH_VTHO_TOPIC)

		events := []*tx.Event{
			{Address: vthoAddr, Topics: []thor.Bytes32{topic}, Data: []byte("event_data_1")},
			{Address: testAddr, Topics: []thor.Bytes32{topic}, Data: []byte("event_data_2")},
		}

		transfers := []*tx.Transfer{
			{Sender: vthoAddr, Recipient: testAddr, Amount: big.NewInt(1000)},
			{Sender: testAddr, Recipient: vthoAddr, Amount: big.NewInt(2000)},
		}

		receipt := &tx.Receipt{
			Outputs: []*tx.Output{
				{Events: events, Transfers: transfers},
			},
		}

		receipts[i] = tx.Receipts{receipt}
	}

	b.ReportAllocs()

	writer := db.NewWriter()
	defer writer.Rollback()

	for i := 0; b.Loop(); i++ {
		err := writer.Write(blocks[i], receipts[i])
		if err != nil {
			b.Fatalf("Write failed: %v", err)
		}

		// Commit every 100 blocks to simulate real usage
		if (i+1)%100 == 0 {
			if err := writer.Commit(); err != nil {
				b.Fatalf("Commit failed: %v", err)
			}
		}
	}

	// Final commit
	if err := writer.Commit(); err != nil {
		b.Fatalf("Final commit failed: %v", err)
	}
}

func BenchmarkPebbleDB_MemoryUsage(b *testing.B) {
	db, cleanup := setupBenchmarkDB(b, 5000) // Large dataset
	defer cleanup()

	vthoAddress := thor.MustParseAddress(BENCH_VTHO_ADDRESS)
	testAddress := thor.MustParseAddress(BENCH_TEST_ADDRESS)
	topic := thor.MustParseBytes32(BENCH_VTHO_TOPIC)
	ctx := context.Background()

	// Complex OR query that could potentially use lots of memory
	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &vthoAddress},
			{Address: &testAddress},
			{Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil}},
		},
		Range:   &logsdb.Range{From: 1, To: 5000},
		Options: &logsdb.Options{Offset: 0, Limit: 50}, // Small limit to test streaming
		Order:   logsdb.ASC,
	}

	// Measure memory before
	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}

		// Force GC to measure peak memory
		if i%10 == 0 {
			runtime.GC()
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	// Report memory usage
	memUsed := memAfter.HeapInuse - memBefore.HeapInuse
	b.ReportMetric(float64(memUsed), "heap-bytes-used")
}

// setupBenchmarkDB creates a test database with sample data
func setupBenchmarkDB(b *testing.B, numBlocks int) (*PebbleDBLogDB, func()) {
	tmpDir, err := os.MkdirTemp("", "pebbledb_bench")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}

	db, err := Open(filepath.Join(tmpDir, "bench.db"))
	if err != nil {
		b.Fatalf("Failed to open database: %v", err)
	}

	writer := db.NewWriter()

	// Create sample data
	vthoAddr := thor.MustParseAddress(BENCH_VTHO_ADDRESS)
	testAddr := thor.MustParseAddress(BENCH_TEST_ADDRESS)
	topic := thor.MustParseBytes32(BENCH_VTHO_TOPIC)

	for blockNum := 1; blockNum <= numBlocks; blockNum++ {
		testBlock := new(block.Builder).
			ParentID(createParentIDForBench(blockNum)).
			Timestamp(uint64(1234567890 + blockNum)).
			TotalScore(uint64(blockNum)).
			GasLimit(10000000).
			Build()

		// Create varied events to ensure good test coverage
		events := []*tx.Event{
			{Address: vthoAddr, Topics: []thor.Bytes32{topic}, Data: []byte("vtho_transfer")},
			{Address: testAddr, Topics: []thor.Bytes32{topic}, Data: []byte("test_event")},
			{Address: vthoAddr, Topics: []thor.Bytes32{}, Data: []byte("vtho_other")}, // No topics
		}

		transfers := []*tx.Transfer{
			{Sender: vthoAddr, Recipient: testAddr, Amount: big.NewInt(int64(blockNum * 1000))},
			{Sender: testAddr, Recipient: vthoAddr, Amount: big.NewInt(int64(blockNum * 500))},
		}

		receipt := &tx.Receipt{
			Outputs: []*tx.Output{
				{Events: events, Transfers: transfers},
			},
		}

		err := writer.Write(testBlock, tx.Receipts{receipt})
		if err != nil {
			b.Fatalf("Failed to write block %d: %v", blockNum, err)
		}

		// Commit every 100 blocks
		if blockNum%100 == 0 {
			if err := writer.Commit(); err != nil {
				b.Fatalf("Failed to commit at block %d: %v", blockNum, err)
			}
		}
	}

	// Final commit
	if err := writer.Commit(); err != nil {
		b.Fatalf("Final commit failed: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// === New Microbenchmarks for PebbleDB Performance Analysis ===

// BenchmarkPebbleDB_AddressOnly_LargeRange tests address-only queries over large block ranges
func BenchmarkPebbleDB_AddressOnly_LargeRange(b *testing.B) {
	resetMetrics() // Reset debug metrics
	defer func() {
		// Only log metrics if debug build (metrics != nil)
		if metrics := getMetrics(); metrics != nil {
			b.Logf("Iterator metrics: Next calls, SeekGE calls, SeekLT calls, Heap operations recorded")
		}
	}()

	db, cleanup := setupBenchmarkDB(b, 10000) // Large dataset
	defer cleanup()

	ctx := context.Background()
	testAddress := thor.MustParseAddress(BENCH_VTHO_ADDRESS)

	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Address: &testAddress}, // Address-only, no topics
		},
		Range:   &logsdb.Range{From: 1000, To: 9000}, // Large range: 8000 blocks
		Options: &logsdb.Options{Offset: 0, Limit: 1000},
		Order:   logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}
	}
}

// BenchmarkPebbleDB_TopicOnly_LargeRange tests topic-only queries over large block ranges
func BenchmarkPebbleDB_TopicOnly_LargeRange(b *testing.B) {
	resetMetrics() // Reset debug metrics
	defer func() {
		// Only log metrics if debug build (metrics != nil)
		if metrics := getMetrics(); metrics != nil {
			b.Logf("Iterator metrics: Next calls, SeekGE calls, SeekLT calls, Heap operations recorded")
		}
	}()

	db, cleanup := setupBenchmarkDB(b, 10000) // Large dataset
	defer cleanup()

	ctx := context.Background()
	topic := thor.MustParseBytes32(BENCH_VTHO_TOPIC)

	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil}}, // Topic0-only
		},
		Range:   &logsdb.Range{From: 1000, To: 9000}, // Large range: 8000 blocks
		Options: &logsdb.Options{Offset: 0, Limit: 1000},
		Order:   logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned")
		}
	}
}

// BenchmarkPebbleDB_MultiTopic_AND tests multi-topic intersection (AND logic)
func BenchmarkPebbleDB_MultiTopic_AND(b *testing.B) {
	resetMetrics() // Reset debug metrics
	defer func() {
		// Only log metrics if debug build (metrics != nil)
		if metrics := getMetrics(); metrics != nil {
			b.Logf("Iterator metrics: Next calls, SeekGE calls, SeekLT calls, Heap operations recorded")
		}
	}()

	db, cleanup := setupBenchmarkDB(b, 5000)
	defer cleanup()

	ctx := context.Background()
	testAddress := thor.MustParseAddress(BENCH_VTHO_ADDRESS)
	topic0 := thor.MustParseBytes32(BENCH_VTHO_TOPIC)

	// Create a second topic for intersection testing
	var topic1Bytes [32]byte
	copy(topic1Bytes[:], []byte("topic1_for_intersection_test"))
	topic1 := thor.Bytes32(topic1Bytes)

	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{
			{
				Address: &testAddress,                                      // Address filter
				Topics:  [5]*thor.Bytes32{&topic0, &topic1, nil, nil, nil}, // Two topics AND
			},
		},
		Range:   &logsdb.Range{From: 1, To: 5000},
		Options: &logsdb.Options{Offset: 0, Limit: 500},
		Order:   logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		// Results may be empty for this intersection test - that's OK
		_ = results
	}
}

// BenchmarkPebbleDB_MassiveOR tests 40-criteria OR queries (stress test union)
func BenchmarkPebbleDB_MassiveOR(b *testing.B) {
	resetMetrics() // Reset debug metrics
	defer func() {
		// Only log metrics if debug build (metrics != nil)
		if metrics := getMetrics(); metrics != nil {
			b.Logf("Iterator metrics: Next calls, SeekGE calls, SeekLT calls, Heap operations recorded")
		}
	}()

	db, cleanup := setupBenchmarkDB(b, 3000) // Moderate dataset for stress test
	defer cleanup()

	ctx := context.Background()

	// Create 40 different criteria to stress test the union
	criteriaSet := make([]*logsdb.EventCriteria, 40)
	for i := range 40 {
		// Generate different addresses to ensure variety
		addr := make([]byte, 8)
		addr[0] = byte(i)      //nolint Make each address different
		addr[1] = byte(i >> 8) //nolint
		address := thor.Address(addr)

		criteriaSet[i] = &logsdb.EventCriteria{
			Address: &address,
		}
	}

	filter := &logsdb.EventFilter{
		CriteriaSet: criteriaSet, // 40 criteria OR
		Range:       &logsdb.Range{From: 1, To: 3000},
		Options:     &logsdb.Options{Offset: 0, Limit: 1000},
		Order:       logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		// Results may be empty - that's OK for this stress test
		_ = results
	}
}

// BenchmarkPebbleDB_EmptyCriteriaSet tests range-only queries (empty CriteriaSet support)
func BenchmarkPebbleDB_EmptyCriteriaSet_Events(b *testing.B) {
	resetMetrics() // Reset debug metrics
	defer func() {
		// Only log metrics if debug build (metrics != nil)
		if metrics := getMetrics(); metrics != nil {
			b.Logf("Iterator metrics: Next calls, SeekGE calls, SeekLT calls, Heap operations recorded")
		}
	}()

	db, cleanup := setupBenchmarkDB(b, 5000)
	defer cleanup()

	ctx := context.Background()

	filter := &logsdb.EventFilter{
		CriteriaSet: []*logsdb.EventCriteria{},           // Empty CriteriaSet - range-only query
		Range:       &logsdb.Range{From: 1000, To: 2000}, // 1000 blocks
		Options:     &logsdb.Options{Offset: 0, Limit: 500},
		Order:       logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterEvents(ctx, filter)
		if err != nil {
			b.Fatalf("FilterEvents failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned for range-only query")
		}
	}
}

// BenchmarkPebbleDB_EmptyCriteriaSet_Transfers tests range-only transfer queries
func BenchmarkPebbleDB_EmptyCriteriaSet_Transfers(b *testing.B) {
	resetMetrics() // Reset debug metrics
	defer func() {
		// Only log metrics if debug build (metrics != nil)
		if metrics := getMetrics(); metrics != nil {
			b.Logf("Iterator metrics: Next calls, SeekGE calls, SeekLT calls, Heap operations recorded")
		}
	}()

	db, cleanup := setupBenchmarkDB(b, 5000)
	defer cleanup()

	ctx := context.Background()

	filter := &logsdb.TransferFilter{
		CriteriaSet: []*logsdb.TransferCriteria{},        // Empty CriteriaSet - range-only query
		Range:       &logsdb.Range{From: 1000, To: 2000}, // 1000 blocks
		Options:     &logsdb.Options{Offset: 0, Limit: 500},
		Order:       logsdb.ASC,
	}

	b.ReportAllocs()

	for b.Loop() {
		results, err := db.FilterTransfers(ctx, filter)
		if err != nil {
			b.Fatalf("FilterTransfers failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("No results returned for range-only transfer query")
		}
	}
}
