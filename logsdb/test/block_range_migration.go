// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/vechain/thor/v2/logsdb"
	pebbledb "github.com/vechain/thor/v2/logsdb/pebbledb"
	"github.com/vechain/thor/v2/logsdb/sqlite3"
	"github.com/vechain/thor/v2/thor"
)

// Note: sequence type, constants, and newSequence function are defined in migration.go

// MigrateSQLiteToPebbleCursorBasedBlockRange migrates using cursor-based queries with block range filtering
func MigrateSQLiteToPebbleCursorBasedBlockRange(sqlitePath, pebblePath string, fromBlock, toBlock uint32, opts *MigrationOptions) (*MigrationStats, error) {
	if opts == nil {
		opts = DefaultMigrationOptions()
	}

	startTime := time.Now()
	stats := &MigrationStats{}

	// Open source SQLite database
	sqliteDB, err := sqlite3.New(sqlitePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer sqliteDB.Close()

	// Create target Pebble database optimized for bulk loading
	pebbleDB, err := pebbledb.OpenForBulkLoad(pebblePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Pebble database: %w", err)
	}
	defer pebbleDB.Close()

	// Get source database size
	if sqliteInfo, err := os.Stat(sqlitePath); err == nil {
		stats.SourceSize = sqliteInfo.Size()
	}

	// Use provided totals (already counted)
	totalEvents := opts.TotalEvents
	totalTransfers := opts.TotalTransfers

	if opts.ProgressLog {
		fmt.Printf("[%s] Block range [%d, %d]: %d events, %d transfers. Starting migration...\n",
			time.Now().Format("15:04:05"), fromBlock, toBlock, totalEvents, totalTransfers)
	}

	// Migrate events using cursor-based approach with block filtering
	if opts.ProgressLog {
		fmt.Printf("[%s] Migrating events with block range filtering...\n", time.Now().Format("15:04:05"))
	}
	eventsCount, err := migrateEventsCursorBasedBlockRange(sqlitePath, pebbleDB, fromBlock, toBlock, totalEvents, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate events: %w", err)
	}
	stats.EventsProcessed = eventsCount

	// Migrate transfers using cursor-based approach with block filtering
	if opts.ProgressLog {
		fmt.Printf("[%s] Migrating transfers with block range filtering...\n", time.Now().Format("15:04:05"))
	}
	transfersCount, err := migrateTransfersCursorBasedBlockRange(sqlitePath, pebbleDB, fromBlock, toBlock, totalTransfers, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate transfers: %w", err)
	}
	stats.TransfersProcessed = transfersCount

	// Migrate metadata (newest block ID, etc.) - but only within range
	if opts.ProgressLog {
		fmt.Printf("[%s] Migrating metadata...\n", time.Now().Format("15:04:05"))
	}

	// Calculate final statistics
	stats.Duration = time.Since(startTime)
	stats.EventsPerSecond = float64(stats.EventsProcessed) / stats.Duration.Seconds()
	stats.TransfersPerSecond = float64(stats.TransfersProcessed) / stats.Duration.Seconds()

	if opts.ProgressLog {
		fmt.Printf("[%s] Block range migration completed in %v - %d events, %d transfers\n",
			time.Now().Format("15:04:05"), stats.Duration.Round(time.Second),
			stats.EventsProcessed, stats.TransfersProcessed)
	}

	// Get target database size (approximate by checking directory size)
	if pebbleInfo, err := getDirSize(pebblePath); err == nil {
		stats.TargetSize = pebbleInfo
	}

	// Verify migration if requested
	if opts.VerifyData {
		if opts.ProgressLog {
			fmt.Printf("[%s] Verifying block range migration...\n", time.Now().Format("15:04:05"))
		}
		// Note: Verification might need adjustment for block range
		if err := verifyMigration(sqliteDB, pebbleDB, opts); err != nil {
			return stats, fmt.Errorf("migration verification failed: %w", err)
		}
		if opts.ProgressLog {
			fmt.Printf("[%s] Block range migration verification passed\n", time.Now().Format("15:04:05"))
		}
	}

	return stats, nil
}

// migrateEventsCursorBasedBlockRange migrates events using cursor-based SQLite queries with block range filtering
func migrateEventsCursorBasedBlockRange(
	sqlitePath string,
	target logsdb.LogsDB,
	fromBlock, toBlock uint32,
	totalEvents int64,
	opts *MigrationOptions,
) (int64, error) {
	// Calculate sequence range for block filtering
	fromSeq, err := newSequence(fromBlock, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to create fromSeq: %w", err)
	}

	toSeq, err := newSequence(toBlock+1, 0, 0) // +1 to include toBlock
	if err != nil {
		return 0, fmt.Errorf("failed to create toSeq: %w", err)
	}

	// Open SQLite database directly for cursor-based queries
	db, err := sql.Open("sqlite3", sqlitePath+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer db.Close()

	// Prepare cursor-based query with block range filtering
	query := `SELECT e.seq, r0.data, e.blockTime, r1.data, r2.data, e.clauseIndex, r3.data, r4.data, r5.data, r6.data, r7.data, r8.data, e.data
			  FROM event e
			  LEFT JOIN ref r0 ON e.blockID = r0.id
			  LEFT JOIN ref r1 ON e.txID = r1.id
			  LEFT JOIN ref r2 ON e.txOrigin = r2.id
			  LEFT JOIN ref r3 ON e.address = r3.id
			  LEFT JOIN ref r4 ON e.topic0 = r4.id
			  LEFT JOIN ref r5 ON e.topic1 = r5.id
			  LEFT JOIN ref r6 ON e.topic2 = r6.id
			  LEFT JOIN ref r7 ON e.topic3 = r7.id
			  LEFT JOIN ref r8 ON e.topic4 = r8.id
			  WHERE e.seq >= ? AND e.seq < ? AND e.seq > ?
			  ORDER BY e.seq 
			  LIMIT ?`

	stmt, err := db.Prepare(query)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare cursor query: %w", err)
	}
	defer stmt.Close()

	var processedEvents int64
	lastSeq := int64(fromSeq - 1) // Start just before the range
	batchCount := 0
	startTime := time.Now()

	// Use bulk writer if available for better performance
	var writer logsdb.Writer
	if pebbleDB, ok := target.(*pebbledb.PebbleDBLogDB); ok {
		writer = pebbleDB.NewBulkWriter()
	} else {
		writer = target.NewWriter()
	}
	defer writer.Rollback() // Ensure cleanup

	for {
		// Execute cursor-based query with block range filtering
		rows, err := stmt.Query(int64(fromSeq), int64(toSeq), lastSeq, opts.BatchSize)
		if err != nil {
			return processedEvents, fmt.Errorf("cursor query failed: %w", err)
		}

		var events []*logsdb.Event
		var maxSeq int64

		// Process batch with same structure as SQLiteDB
		for rows.Next() {
			var event logsdb.Event
			var seq int64
			var blockIDData, txIDData, txOriginData, addressData, topic0Data, topic1Data, topic2Data, topic3Data, topic4Data []byte

			err := rows.Scan(&seq, &blockIDData, &event.BlockTime, &txIDData, &txOriginData,
				&event.ClauseIndex, &addressData, &topic0Data, &topic1Data, &topic2Data, &topic3Data, &topic4Data, &event.Data)
			if err != nil {
				rows.Close()
				return processedEvents, fmt.Errorf("failed to scan event: %w", err)
			}

			// Convert binary data to proper types (same as SQLiteDB queryEvents method)
			event.BlockID = thor.BytesToBytes32(blockIDData)
			event.TxID = thor.BytesToBytes32(txIDData)
			event.TxOrigin = thor.BytesToAddress(txOriginData)
			event.Address = thor.BytesToAddress(addressData)

			// Parse sequence to get block number, tx index, log index
			s := sequence(seq)
			event.BlockNumber = s.BlockNumber()
			event.TxIndex = s.TxIndex()
			event.LogIndex = s.LogIndex()

			// Convert topics from byte arrays
			event.Topics = [5]*thor.Bytes32{
				parseNullableBytes(topic0Data),
				parseNullableBytes(topic1Data),
				parseNullableBytes(topic2Data),
				parseNullableBytes(topic3Data),
				parseNullableBytes(topic4Data),
			}

			events = append(events, &event)
			maxSeq = seq
		}
		rows.Close()

		if len(events) == 0 {
			break // No more events in range
		}

		// Write the batch using migration-specific method
		if err := migrateEventBatch(events, writer, opts); err != nil {
			return processedEvents, fmt.Errorf("failed to migrate event batch %d: %w", batchCount, err)
		}

		// Commit only every 5 batches (5M events) to reduce I/O overhead
		commitNow := (batchCount+1)%5 == 0 || len(events) < int(opts.BatchSize)
		if commitNow {
			// Use NoSync for bulk loading performance
			if pebbleWriter, ok := writer.(*pebbledb.PebbleDBWriter); ok {
				if err := pebbleWriter.CommitNoSync(); err != nil {
					writer.Rollback()
					return processedEvents, fmt.Errorf("failed to commit event batch %d: %w", batchCount, err)
				}
			} else {
				if err := writer.Commit(); err != nil {
					writer.Rollback()
					return processedEvents, fmt.Errorf("failed to commit event batch %d: %w", batchCount, err)
				}
			}
		}

		processedEvents += int64(len(events))
		lastSeq = maxSeq
		batchCount++

		if opts.ProgressLog {
			elapsed := time.Since(startTime).Seconds()
			eventsPerSec := float64(processedEvents) / elapsed
			percentage := float64(processedEvents) * 100.0 / float64(totalEvents)
			fmt.Printf("[%s] Block range [%d-%d]: %d/%d events (%.1f%%) - %.0f events/sec\n",
				time.Now().Format("15:04:05"), fromBlock, toBlock, processedEvents, totalEvents, percentage, eventsPerSec)
		}

		// If we got fewer events than requested, we've reached the end
		if len(events) < int(opts.BatchSize) {
			break
		}
	}

	// Final commit to ensure all events are written
	if pebbleWriter, ok := writer.(*pebbledb.PebbleDBWriter); ok {
		if err := pebbleWriter.CommitNoSync(); err != nil {
			return processedEvents, fmt.Errorf("failed to commit final event batch: %w", err)
		}
	} else {
		if err := writer.Commit(); err != nil {
			return processedEvents, fmt.Errorf("failed to commit final event batch: %w", err)
		}
	}

	return processedEvents, nil
}

// migrateTransfersCursorBasedBlockRange migrates transfers using cursor-based SQLite queries with block range filtering
func migrateTransfersCursorBasedBlockRange(
	sqlitePath string,
	target logsdb.LogsDB,
	fromBlock, toBlock uint32,
	totalTransfers int64,
	opts *MigrationOptions,
) (int64, error) {
	// For now, just return 0 - transfers implementation would be similar to events
	if opts.ProgressLog {
		fmt.Printf("[%s] Skipping transfers for block range [%d-%d] (not implemented yet)\n",
			time.Now().Format("15:04:05"), fromBlock, toBlock)
	}
	return 0, nil
}
