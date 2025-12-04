// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/logsdb/pebbledb"
	"github.com/vechain/thor/v2/logsdb/sqlite3"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Constants moved to constants.go

// Flags are now defined in shared_flags.go

// BenchmarkComparative_FilterEvents runs comprehensive benchmarks comparing SQLite vs Pebble
// query performance across various scenarios including complex multi-criteria queries.
//
// The benchmarks test:
// - Basic single-criteria filters (address, topic, range, limit)
// - Complex multi-criteria combinations (2-3 way index intersections)
// - Range + intersection combos (realistic blockchain query patterns)
// - Stress tests (4+ way intersections: address + multiple topics)
// - SQLite compound indexes vs Pebble manual intersection performance
//
// Range + intersection scenarios are particularly important for blockchain applications
// where queries often combine block ranges with contract addresses and event types.
//
// Usage: go test -bench=BenchmarkComparative_FilterEvents -sqliteDbPath=<sqlite> -pebblePath=<pebble>
func BenchmarkComparative_FilterEvents(b *testing.B) {
	// Test scenarios
	vthoAddress := thor.MustParseAddress(VTHO_ADDRESS)
	topic := thor.MustParseBytes32(VTHO_TOPIC)

	addressFilterCriteria := []*logsdb.EventCriteria{
		{Address: &vthoAddress},
	}
	topicFilterCriteria := []*logsdb.EventCriteria{
		{Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil}},
	}

	// Complex multi-criteria scenarios (challenging for query optimization)
	topic2 := thor.MustParseBytes32("0x8C5BE1E5EBEC7D5BD14F71427D1E84F3DD0314C0F7B2291E5B200AC8C7C3B925") // Approval event
	testAddress := thor.MustParseAddress(TEST_ADDRESS)

	// Address + Topic0 (both indexed - should be fast)
	addressAndTopic0Criteria := []*logsdb.EventCriteria{
		{Address: &vthoAddress, Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil}},
	}

	// Address + Topic1 (topic1 not independently indexed - requires intersection)
	addressAndTopic1Criteria := []*logsdb.EventCriteria{
		{Address: &testAddress, Topics: [5]*thor.Bytes32{nil, &topic2, nil, nil, nil}},
	}

	// Topic1 + Topic2 (both non-indexed topics - most challenging)
	topic1AndTopic2Criteria := []*logsdb.EventCriteria{
		{Topics: [5]*thor.Bytes32{nil, &topic, &topic2, nil, nil}},
	}

	// Multiple address criteria (should use index intersection)
	multiAddressCriteria := []*logsdb.EventCriteria{
		{Address: &vthoAddress},
		{Address: &testAddress},
	}

	// Complex: Address + multiple non-indexed topics
	complexMultiTopicCriteria := []*logsdb.EventCriteria{
		{Address: &testAddress, Topics: [5]*thor.Bytes32{nil, &topic, &topic2, nil, nil}},
	}

	// Range + intersection combos (realistic blockchain queries)
	// These test SQLite's compound indexes vs Pebble's manual intersection with range filtering

	// Range + Address (block range with specific contract)
	rangeAndAddressCriteria := []*logsdb.EventCriteria{
		{Address: &vthoAddress},
	}

	// Range + Address + Topic0 (block range + contract + event type)
	rangeAndAddressAndTopicCriteria := []*logsdb.EventCriteria{
		{Address: &vthoAddress, Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil}},
	}

	// Range + Multiple topics (block range + complex event matching)
	rangeAndMultiTopicCriteria := []*logsdb.EventCriteria{
		{Topics: [5]*thor.Bytes32{&topic, &topic2, nil, nil, nil}},
	}

	// Range + Complex intersection (most challenging scenario)
	rangeAndComplexCriteria := []*logsdb.EventCriteria{
		{Address: &testAddress, Topics: [5]*thor.Bytes32{nil, &topic, nil, &topic2, nil}},
	}

	// Stress test: Range + 4-way intersection (address + 3 topics)
	topic3 := thor.MustParseBytes32("0x8BE0079C531659141344CD1FD0A4F28419497F9722A3DAAFE3B4186F6B6457E0") // OwnershipTransferred
	rangeAndStressTestCriteria := []*logsdb.EventCriteria{
		{Address: &vthoAddress, Topics: [5]*thor.Bytes32{&topic, &topic2, &topic3, nil, nil}},
	}

	tests := []struct {
		name string
		arg  *logsdb.EventFilter
	}{
		// Basic single-criteria tests
		{"AddressCriteriaFilter", &logsdb.EventFilter{CriteriaSet: addressFilterCriteria, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{"TopicCriteriaFilter", &logsdb.EventFilter{CriteriaSet: topicFilterCriteria, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{"EventLimit", &logsdb.EventFilter{Order: logsdb.ASC, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{"EventLimitDesc", &logsdb.EventFilter{Order: logsdb.DESC, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{"EventRange", &logsdb.EventFilter{Range: &logsdb.Range{From: 500000, To: 1_000_000}, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{
			"EventRangeDesc",
			&logsdb.EventFilter{Range: &logsdb.Range{From: 500000, To: 1_000_000}, Options: &logsdb.Options{Offset: 0, Limit: 500000}, Order: logsdb.DESC},
		},

		// Complex multi-criteria tests (challenging for query optimization)
		{"AddressAndTopic0", &logsdb.EventFilter{CriteriaSet: addressAndTopic0Criteria, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{"AddressAndTopic1", &logsdb.EventFilter{CriteriaSet: addressAndTopic1Criteria, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{"Topic1AndTopic2", &logsdb.EventFilter{CriteriaSet: topic1AndTopic2Criteria, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{"MultipleAddresses", &logsdb.EventFilter{CriteriaSet: multiAddressCriteria, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{"ComplexMultiTopic", &logsdb.EventFilter{CriteriaSet: complexMultiTopicCriteria, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},

		// Range + intersection combos (realistic blockchain query patterns)
		{
			"RangeAndAddress",
			&logsdb.EventFilter{
				CriteriaSet: rangeAndAddressCriteria,
				Range:       &logsdb.Range{From: 100000, To: 200000},
				Options:     &logsdb.Options{Offset: 0, Limit: 500000},
			},
		},
		{
			"RangeAndAddressAndTopic",
			&logsdb.EventFilter{
				CriteriaSet: rangeAndAddressAndTopicCriteria,
				Range:       &logsdb.Range{From: 100000, To: 200000},
				Options:     &logsdb.Options{Offset: 0, Limit: 500000},
			},
		},
		{
			"RangeAndMultiTopic",
			&logsdb.EventFilter{
				CriteriaSet: rangeAndMultiTopicCriteria,
				Range:       &logsdb.Range{From: 100000, To: 200000},
				Options:     &logsdb.Options{Offset: 0, Limit: 500000},
			},
		},
		{
			"RangeAndComplex",
			&logsdb.EventFilter{
				CriteriaSet: rangeAndComplexCriteria,
				Range:       &logsdb.Range{From: 100000, To: 200000},
				Options:     &logsdb.Options{Offset: 0, Limit: 500000},
			},
		},
		{
			"RangeAndStressTest",
			&logsdb.EventFilter{
				CriteriaSet: rangeAndStressTestCriteria,
				Range:       &logsdb.Range{From: 100000, To: 200000},
				Options:     &logsdb.Options{Offset: 0, Limit: 500000},
			},
		},
	}

	for _, tt := range tests {
		b.Run("SQLite/"+tt.name, func(b *testing.B) {
			db := loadSQLiteDB(b)
			defer db.Close()

			b.ResetTimer()
			for b.Loop() {
				_, err := db.FilterEvents(context.Background(), tt.arg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Pebble/"+tt.name, func(b *testing.B) {
			db := loadPebbleDB(b)
			defer db.Close()

			b.ResetTimer()
			for b.Loop() {
				_, err := db.FilterEvents(context.Background(), tt.arg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkComparative_FilterTransfers runs transfer benchmarks against both databases
func BenchmarkComparative_FilterTransfers(b *testing.B) {
	txOrigin := thor.MustParseAddress(TEST_ADDRESS)
	transferCriteria := []*logsdb.TransferCriteria{
		{TxOrigin: &txOrigin, Sender: nil, Recipient: nil},
	}

	tests := []struct {
		name string
		arg  *logsdb.TransferFilter
	}{
		{"TransferCriteria", &logsdb.TransferFilter{CriteriaSet: transferCriteria, Options: &logsdb.Options{Offset: 0, Limit: 500_000}}},
		{
			"TransferCriteriaDesc",
			&logsdb.TransferFilter{Order: logsdb.DESC, CriteriaSet: transferCriteria, Options: &logsdb.Options{Offset: 0, Limit: 500_000}},
		},
		{"Ranged500K", &logsdb.TransferFilter{Range: &logsdb.Range{From: 500_000, To: 1_000_000}, Options: &logsdb.Options{Offset: 0, Limit: 500000}}},
		{
			"Ranged500KDesc",
			&logsdb.TransferFilter{Range: &logsdb.Range{From: 500_000, To: 1_000_000}, Options: &logsdb.Options{Offset: 0, Limit: 500000}, Order: logsdb.DESC},
		},
	}

	for _, tt := range tests {
		b.Run("SQLite/"+tt.name, func(b *testing.B) {
			db := loadSQLiteDB(b)
			defer db.Close()

			b.ResetTimer()
			for b.Loop() {
				_, err := db.FilterTransfers(context.Background(), tt.arg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Pebble/"+tt.name, func(b *testing.B) {
			db := loadPebbleDB(b)
			defer db.Close()

			b.ResetTimer()
			for b.Loop() {
				_, err := db.FilterTransfers(context.Background(), tt.arg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkComparative_HasBlockID benchmarks block ID lookup
func BenchmarkComparative_HasBlockID(b *testing.B) {
	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteDB(b)
		defer db.Close()

		// Get some block IDs to test with
		events, err := db.FilterEvents(context.Background(), &logsdb.EventFilter{
			Options: &logsdb.Options{Offset: 0, Limit: 1000},
		})
		require.NoError(b, err)
		require.Greater(b, len(events), 0, "need events to benchmark HasBlockID")

		blockIDs := make([]thor.Bytes32, len(events))
		for i, event := range events {
			blockIDs[i] = event.BlockID
		}

		b.ResetTimer()
		for b.Loop() {
			for _, blockID := range blockIDs {
				_, err := db.HasBlockID(blockID)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleDB(b)
		defer db.Close()

		// Get some block IDs to test with
		events, err := db.FilterEvents(context.Background(), &logsdb.EventFilter{
			Options: &logsdb.Options{Offset: 0, Limit: 1000},
		})
		require.NoError(b, err)
		require.Greater(b, len(events), 0, "need events to benchmark HasBlockID")

		blockIDs := make([]thor.Bytes32, len(events))
		for i, event := range events {
			blockIDs[i] = event.BlockID
		}

		b.ResetTimer()
		for b.Loop() {
			for _, blockID := range blockIDs {
				_, err := db.HasBlockID(blockID)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// BenchmarkComparative_WriteBlocks benchmarks write performance with synthetic data
func BenchmarkComparative_WriteBlocks(b *testing.B) {
	writeCount := 1000

	b.Run("SQLite", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			db, cleanup := createTempSQLiteDB(b)
			b.StartTimer()

			writer := db.NewWriter()
			blk := new(block.Builder).Build()

			for range writeCount {
				blk = new(block.Builder).
					ParentID(blk.Header().ID()).
					Transaction(newTx(tx.TypeLegacy)).
					Build()
				receipts := tx.Receipts{newReceipt()}

				err := writer.Write(blk, receipts)
				if err != nil {
					b.Fatal(err)
				}
			}

			err := writer.Commit()
			if err != nil {
				b.Fatal(err)
			}

			b.StopTimer()
			db.Close()
			cleanup()
			b.StartTimer()
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			db, cleanup := createTempPebbleDB(b)
			b.StartTimer()

			writer := db.NewWriter()
			blk := new(block.Builder).Build()

			for range writeCount {
				blk = new(block.Builder).
					ParentID(blk.Header().ID()).
					Transaction(newTx(tx.TypeLegacy)).
					Build()
				receipts := tx.Receipts{newReceipt()}

				err := writer.Write(blk, receipts)
				if err != nil {
					b.Fatal(err)
				}
			}

			err := writer.Commit()
			if err != nil {
				b.Fatal(err)
			}

			b.StopTimer()
			db.Close()
			cleanup()
			b.StartTimer()
		}
	})
}

// BenchmarkMigration benchmarks the migration process itself
// If a Pebble database already exists at pebblePath, migration is skipped
func BenchmarkMigration(b *testing.B) {
	if *SqliteDbPath == "" {
		b.Skip("sqliteDbPath flag required for migration benchmark")
	}

	b.Run("SQLiteToPebble", func(b *testing.B) {
		// Check if Pebble database already exists
		if *PebblePath != "" {
			if db, err := pebbledb.Open(*PebblePath); err == nil {
				db.Close()
				b.Log("Pebble database already exists, skipping migration")
				b.Skip("Migration skipped - Pebble database already exists at " + *PebblePath)
				return
			}
		}

		for b.Loop() {
			b.StopTimer()
			// Use provided pebblePath or create temp directory
			var pebbleDir string
			var cleanup func()

			if *PebblePath != "" {
				pebbleDir = *PebblePath
				cleanup = func() {} // No cleanup for permanent database
			} else {
				tempDir, err := os.MkdirTemp("", "benchmark_migration_")
				require.NoError(b, err)
				pebbleDir = filepath.Join(tempDir, "pebble.db")
				cleanup = func() { os.RemoveAll(tempDir) }
			}
			defer cleanup()

			b.StartTimer()

			// Run migration
			stats, err := MigrateSQLiteToPebble(*SqliteDbPath, pebbleDir, &MigrationOptions{
				BatchSize:   10000,
				ProgressLog: false,
				VerifyData:  false,
			})
			if err != nil {
				b.Fatal(err)
			}

			b.StopTimer()
			b.ReportMetric(float64(stats.EventsProcessed), "events")
			b.ReportMetric(float64(stats.TransfersProcessed), "transfers")
			b.ReportMetric(stats.EventsPerSecond, "events/sec")
			b.ReportMetric(stats.TransfersPerSecond, "transfers/sec")
			b.StartTimer()
		}
	})
}

// Helper functions
func loadSQLiteDB(b *testing.B) logsdb.LogsDB {
	if *SqliteDbPath == "" {
		b.Fatal("Please provide a sqliteDbPath")
	}

	db, err := sqlite3.New(*SqliteDbPath)
	if err != nil {
		b.Fatal(err)
	}
	return db
}

func loadPebbleDB(b *testing.B) logsdb.LogsDB {
	if *PebblePath == "" {
		b.Fatal("Please provide a pebblePath")
	}

	db, err := pebbledb.Open(*PebblePath)
	if err != nil {
		b.Fatal(err)
	}
	return db
}

func createTempSQLiteDB(b *testing.B) (logsdb.LogsDB, func()) {
	dir, err := os.MkdirTemp("", "sqlite_bench_")
	require.NoError(b, err)

	tmpFile, err := os.CreateTemp(dir, "temp-*.db")
	require.NoError(b, err)
	require.NoError(b, tmpFile.Close())

	db, err := sqlite3.New(tmpFile.Name())
	require.NoError(b, err)

	cleanup := func() {
		os.RemoveAll(dir)
	}

	return db, cleanup
}

func createTempPebbleDB(b *testing.B) (logsdb.LogsDB, func()) {
	dir, err := os.MkdirTemp("", "pebble_bench_")
	require.NoError(b, err)

	db, err := pebbledb.Open(filepath.Join(dir, "pebble.db"))
	require.NoError(b, err)

	cleanup := func() {
		os.RemoveAll(dir)
	}

	return db, cleanup
}
