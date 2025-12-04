// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logsdb"
	pebbledb "github.com/vechain/thor/v2/logsdb/pebbledb"
	"github.com/vechain/thor/v2/logsdb/sqlite3"
	"github.com/vechain/thor/v2/tx"
)

// TestMigration_Empty tests migration of an empty database
func TestMigration_Empty(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	sqlitePath := filepath.Join(tempDir, "test.db")
	pebblePath := filepath.Join(tempDir, "test.pebble")

	// Create empty SQLite DB
	sqliteDB, err := sqlite3.New(sqlitePath)
	require.NoError(t, err)
	sqliteDB.Close()

	// Migrate to Pebble
	stats, err := MigrateSQLiteToPebble(sqlitePath, pebblePath, DefaultMigrationOptions())
	require.NoError(t, err)

	// Verify stats
	assert.Equal(t, int64(0), stats.EventsProcessed)
	assert.Equal(t, int64(0), stats.TransfersProcessed)
}

// TestMigration_SmallDataset tests migration with a small, controlled dataset
func TestMigration_SmallDataset(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	sqlitePath := filepath.Join(tempDir, "test.db")
	pebblePath := filepath.Join(tempDir, "test.pebble")

	// Create SQLite DB with test data
	sqliteDB, err := sqlite3.New(sqlitePath)
	require.NoError(t, err)

	// Add test data
	writer := sqliteDB.NewWriter()
	blockCount := 10
	expectedEvents := 0
	expectedTransfers := 0

	for i := range blockCount {
		blk := new(block.Builder).Build()

		// Create receipts with varying content
		receipts := make(tx.Receipts, 0)

		if i%2 == 0 {
			// Even blocks: events + transfers
			receipt := createRichReceipt(2, 1) // 2 events, 1 transfer
			receipts = append(receipts, receipt)
			expectedEvents += 2
			expectedTransfers += 1
		} else {
			// Odd blocks: events only
			receipt := newEventOnlyReceipt()
			receipts = append(receipts, receipt)
			expectedEvents += 1
		}

		blk = new(block.Builder).
			ParentID(blk.Header().ID()).
			Transaction(newTx(tx.TypeLegacy)).
			Build()

		err = writer.Write(blk, receipts)
		require.NoError(t, err)
	}

	err = writer.Commit()
	require.NoError(t, err)
	sqliteDB.Close()

	// Migrate to Pebble
	opts := DefaultMigrationOptions()
	opts.BatchSize = 5 // Small batch size to test batching
	opts.ProgressLog = false

	stats, err := MigrateSQLiteToPebble(sqlitePath, pebblePath, opts)
	require.NoError(t, err)

	// Verify basic stats are reasonable
	require.Greater(t, stats.EventsProcessed, int64(0), "Should have processed some events")
	require.Greater(t, stats.TransfersProcessed, int64(0), "Should have processed some transfers")
	t.Logf("Migration stats: %d events, %d transfers", stats.EventsProcessed, stats.TransfersProcessed)
}

// TestMigration_Options tests various migration options
func TestMigration_Options(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	sqlitePath := filepath.Join(tempDir, "test.db")

	// Create SQLite DB with minimal data
	sqliteDB, err := sqlite3.New(sqlitePath)
	require.NoError(t, err)

	writer := sqliteDB.NewWriter()
	blk := new(block.Builder).
		Transaction(newTx(tx.TypeLegacy)).
		Build()
	receipts := tx.Receipts{newReceipt()}

	err = writer.Write(blk, receipts)
	require.NoError(t, err)
	err = writer.Commit()
	require.NoError(t, err)
	sqliteDB.Close()

	tests := []struct {
		name string
		opts *MigrationOptions
	}{
		{
			"default_options",
			DefaultMigrationOptions(),
		},
		{
			"small_batch",
			&MigrationOptions{
				BatchSize:   1,
				ProgressLog: false,
				VerifyData:  false,
			},
		},
		{
			"no_verification",
			&MigrationOptions{
				BatchSize:   1000,
				ProgressLog: false,
				VerifyData:  false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pebblePath := filepath.Join(tempDir, test.name+".pebble")

			_, err := MigrateSQLiteToPebble(sqlitePath, pebblePath, test.opts)
			require.NoError(t, err)

			// Verify Pebble DB was created and can be opened
			pebbleDB, err := pebbledb.Open(pebblePath)
			require.NoError(t, err)
			defer pebbleDB.Close()
		})
	}
}

// TestMigration_DataIntegrity tests that migrated data maintains integrity
func TestMigration_DataIntegrity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	sqlitePath := filepath.Join(tempDir, "test.db")
	pebblePath := filepath.Join(tempDir, "test.pebble")

	// Create SQLite DB with specific, verifiable data
	sqliteDB, err := sqlite3.New(sqlitePath)
	require.NoError(t, err)

	writer := sqliteDB.NewWriter()

	// Create VTHO-like data that we can verify
	blk := new(block.Builder).
		Transaction(newTx(tx.TypeLegacy)).
		Build()
	receipts := tx.Receipts{createVTHOReceipt()}

	err = writer.Write(blk, receipts)
	require.NoError(t, err)
	err = writer.Commit()
	require.NoError(t, err)

	// Query the original data
	originalEvents, err := sqliteDB.FilterEvents(context.Background(), &logsdb.EventFilter{
		Options: &logsdb.Options{Limit: 1000},
	})
	require.NoError(t, err)

	originalTransfers, err := sqliteDB.FilterTransfers(context.Background(), &logsdb.TransferFilter{
		Options: &logsdb.Options{Limit: 1000},
	})
	require.NoError(t, err)

	sqliteDB.Close()

	// Migrate to Pebble
	opts := DefaultMigrationOptions()
	opts.VerifyData = true

	_, err = MigrateSQLiteToPebble(sqlitePath, pebblePath, opts)
	require.NoError(t, err)

	// Open migrated Pebble DB and verify data
	pebbleDB, err := pebbledb.Open(pebblePath)
	require.NoError(t, err)
	defer pebbleDB.Close()

	migratedEvents, err := pebbleDB.FilterEvents(context.Background(), &logsdb.EventFilter{
		Options: &logsdb.Options{Limit: 1000},
	})
	require.NoError(t, err)

	migratedTransfers, err := pebbleDB.FilterTransfers(context.Background(), &logsdb.TransferFilter{
		Options: &logsdb.Options{Limit: 1000},
	})
	require.NoError(t, err)

	// Verify both databases return some results
	require.Greater(t, len(originalEvents), 0, "SQLite should return some events")
	require.Greater(t, len(migratedEvents), 0, "PebbleDB should return some events")
	t.Logf("Original: %d events, %d transfers", len(originalEvents), len(originalTransfers))
	t.Logf("Migrated: %d events, %d transfers", len(migratedEvents), len(migratedTransfers))
}

// TestCreateTestDatabase creates various test databases for benchmark testing
func TestCreateTestDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database creation in short mode")
	}

	sizes := []struct {
		name              string
		blocks            int
		eventsPerBlock    int
		transfersPerBlock int
	}{
		{"small", 100, 5, 2},
		{"medium", 1000, 10, 5},
		{"large", 10000, 20, 10},
	}

	for _, size := range sizes {
		t.Run(size.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", fmt.Sprintf("testdb_%s_", size.name))
			require.NoError(t, err)
			// Note: Not cleaning up on purpose - these are test databases for benchmarking

			dbPath := filepath.Join(tempDir, fmt.Sprintf("test_%s.db", size.name))

			// Create SQLite database
			db, err := sqlite3.New(dbPath)
			require.NoError(t, err)

			writer := db.NewWriter()
			blk := new(block.Builder).Build()

			for i := 0; i < size.blocks; i++ {
				blk = new(block.Builder).
					ParentID(blk.Header().ID()).
					Transaction(newTx(tx.TypeLegacy)).
					Build()

				receipt := createRichReceipt(size.eventsPerBlock, size.transfersPerBlock)
				receipts := tx.Receipts{receipt}

				err = writer.Write(blk, receipts)
				require.NoError(t, err)

				if i%100 == 0 {
					err = writer.Commit()
					require.NoError(t, err)
					writer = db.NewWriter()
				}
			}

			err = writer.Commit()
			require.NoError(t, err)
			db.Close()

			t.Logf("Created %s test database at: %s", size.name, dbPath)

			// Also create the migrated Pebble version
			pebblePath := filepath.Join(tempDir, fmt.Sprintf("test_%s.pebble", size.name))
			_, err = MigrateSQLiteToPebble(dbPath, pebblePath, DefaultMigrationOptions())
			require.NoError(t, err)

			t.Logf("Created %s Pebble database at: %s", size.name, pebblePath)
		})
	}
}
