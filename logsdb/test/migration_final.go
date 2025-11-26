// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/logsdb/pebbledb"
	"github.com/vechain/thor/v2/logsdb/sqlite3"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// === Core Migration Types ===

// sequence type for parsing SQLite seq values (matches PebbleV3 implementation)
type sequence int64

// Exact same bit allocation as PebbleV3 implementation for compatibility
const (
	blockNumBits = 28
	txIndexBits  = 15
	logIndexBits = 20
	// Max = 2^28 - 1 = 268,435,455 (unsigned int 28)
	blockNumMask = (1 << blockNumBits) - 1
	// Max = 2^15 - 1 = 32,767
	txIndexMask = (1 << txIndexBits) - 1
	// Max = 2^20 - 1 = 1,048,575
	logIndexMask = (1 << logIndexBits) - 1
)

func (s sequence) BlockNumber() uint32 {
	return uint32(s>>(txIndexBits+logIndexBits)) & blockNumMask
}

func (s sequence) TxIndex() uint32 {
	return uint32((s >> logIndexBits) & txIndexMask)
}

func (s sequence) LogIndex() uint32 {
	return uint32(s & logIndexMask)
}

// newSequence creates a new sequence from block number, tx index, and log index
func newSequence(blockNum uint32, txIndex uint32, logIndex uint32) (sequence, error) {
	if blockNum > blockNumMask {
		return 0, fmt.Errorf("block number out of range: uint28")
	}
	if txIndex > txIndexMask {
		return 0, fmt.Errorf("tx index out of range: uint15")
	}
	if logIndex > logIndexMask {
		return 0, fmt.Errorf("log index out of range: uint20")
	}

	return (sequence(blockNum) << (txIndexBits + logIndexBits)) |
		(sequence(txIndex) << logIndexBits) |
		sequence(logIndex), nil
}

// MigrationStats tracks migration progress and performance metrics
type MigrationStats struct {
	EventsProcessed    int64         `json:"eventsProcessed"`    // Total events migrated
	TransfersProcessed int64         `json:"transfersProcessed"` // Total transfers migrated
	Duration           time.Duration `json:"duration"`           // Total migration time
	SourceSize         int64         `json:"sourceSizeBytes"`    // Source database size
	TargetSize         int64         `json:"targetSizeBytes"`    // Target database size
	EventsPerSecond    float64       `json:"eventsPerSecond"`    // Event processing rate
	TransfersPerSecond float64       `json:"transfersPerSecond"` // Transfer processing rate
}

// MigrationOptions configures the migration process
type MigrationOptions struct {
	BatchSize      uint64 `json:"batchSize"`      // Number of events/transfers to process per batch
	ProgressLog    bool   `json:"progressLog"`    // Whether to log progress during migration
	VerifyData     bool   `json:"verifyData"`     // Whether to verify data integrity after migration
	TotalEvents    int64  `json:"totalEvents"`    // Pre-known total events count (0 = auto-count)
	TotalTransfers int64  `json:"totalTransfers"` // Pre-known total transfers count (0 = auto-count)
}

// DefaultMigrationOptions returns sensible defaults for production migration
func DefaultMigrationOptions() *MigrationOptions {
	return &MigrationOptions{
		BatchSize:   200000, // Optimized batch size for high performance
		ProgressLog: true,   // Enable progress tracking for long migrations
		VerifyData:  true,   // Verify data integrity by default
	}
}

// FastMigrationOptions returns optimized settings for maximum migration speed
func FastMigrationOptions() *MigrationOptions {
	return &MigrationOptions{
		BatchSize:   1000000, // 1M events per batch - 5x larger batches
		ProgressLog: true,    // Keep progress tracking
		VerifyData:  false,   // Skip verification for maximum speed
	}
}

// === Main Migration Functions ===

// MigrateSQLiteToPebble migrates all data from a SQLite logsdb to PebbleV3 logsdb
func MigrateSQLiteToPebble(sqlitePath, pebblePath string, opts *MigrationOptions) (*MigrationStats, error) {
	return MigrateSQLiteToPebbleCursorBased(sqlitePath, pebblePath, opts)
}

// MigrateSQLiteToPebbleFast performs fast migration with optimized settings
func MigrateSQLiteToPebbleFast(sqlitePath, pebblePath string) (*MigrationStats, error) {
	return MigrateSQLiteToPebbleCursorBased(sqlitePath, pebblePath, FastMigrationOptions())
}

// MigrateSQLiteToPebbleUltraFast performs maximum-speed migration using cursor-based SQLite reading
func MigrateSQLiteToPebbleUltraFast(sqlitePath, pebblePath string) (*MigrationStats, error) {
	return MigrateSQLiteToPebbleCursorBased(sqlitePath, pebblePath, FastMigrationOptions())
}

// MigrateSQLiteToPebbleUltraFastWithTotals performs maximum-speed migration with pre-known totals
func MigrateSQLiteToPebbleUltraFastWithTotals(sqlitePath, pebblePath string, eventCount, transferCount int64) (*MigrationStats, error) {
	opts := FastMigrationOptions()
	opts.TotalEvents = eventCount
	opts.TotalTransfers = transferCount
	return MigrateSQLiteToPebbleCursorBased(sqlitePath, pebblePath, opts)
}

// MigrateSQLiteToPebbleCursorBased performs the core cursor-based migration to PebbleV3
func MigrateSQLiteToPebbleCursorBased(sqlitePath, pebblePath string, opts *MigrationOptions) (*MigrationStats, error) {
	if opts == nil {
		opts = DefaultMigrationOptions()
	}

	startTime := time.Now()
	stats := &MigrationStats{}

	// Create target PebbleV3 database for bulk loading
	pebbleDB, err := pebbledb.OpenForBulkLoad(pebblePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create PebbleV3 database: %w", err)
	}
	defer pebbleDB.Close()

	// Count total records if not provided
	var totalEvents, totalTransfers int64
	if opts.TotalEvents > 0 && opts.TotalTransfers > 0 {
		totalEvents = opts.TotalEvents
		totalTransfers = opts.TotalTransfers
		if opts.ProgressLog {
			fmt.Printf("[%s] Using pre-known totals: %d events, %d transfers\n",
				time.Now().Format("15:04:05"), totalEvents, totalTransfers)
		}
	} else {
		if opts.ProgressLog {
			fmt.Printf("[%s] Counting total events and transfers...\n", time.Now().Format("15:04:05"))
		}
		totalEvents, totalTransfers, err = countTotalRecordsFast(sqlitePath)
		if err != nil {
			return nil, fmt.Errorf("failed to count records: %w", err)
		}
		if opts.ProgressLog {
			fmt.Printf("[%s] Found %d events, %d transfers. Starting migration...\n",
				time.Now().Format("15:04:05"), totalEvents, totalTransfers)
		}
	}

	// Migrate events
	if opts.ProgressLog {
		fmt.Printf("[%s] Migrating events...\n", time.Now().Format("15:04:05"))
	}
	eventsProcessed, err := migrateEventsCursorBased(sqlitePath, pebbleDB, totalEvents, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate events: %w", err)
	}
	stats.EventsProcessed = eventsProcessed

	// Migrate transfers
	if opts.ProgressLog {
		fmt.Printf("[%s] Migrating transfers...\n", time.Now().Format("15:04:05"))
	}
	transfersProcessed, err := migrateTransfersCursorBased(sqlitePath, pebbleDB, totalTransfers, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate transfers: %w", err)
	}
	stats.TransfersProcessed = transfersProcessed

	// Migrate metadata
	if opts.ProgressLog {
		fmt.Printf("[%s] Migrating metadata...\n", time.Now().Format("15:04:05"))
	}
	sourceDB, err := sqlite3.New(sqlitePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source database for metadata: %w", err)
	}
	defer sourceDB.Close()

	// Calculate final stats
	stats.Duration = time.Since(startTime)
	if sourceSize, err := getDirSize(sqlitePath); err == nil {
		stats.SourceSize = sourceSize
	}
	if pebbleInfo, err := getDirSize(pebblePath); err == nil {
		stats.TargetSize = pebbleInfo
	}

	if stats.Duration.Seconds() > 0 {
		stats.EventsPerSecond = float64(stats.EventsProcessed) / stats.Duration.Seconds()
		stats.TransfersPerSecond = float64(stats.TransfersProcessed) / stats.Duration.Seconds()
	}

	if opts.ProgressLog {
		fmt.Printf("[%s] Migration completed in %v - %d events, %d transfers\n",
			time.Now().Format("15:04:05"), stats.Duration, stats.EventsProcessed, stats.TransfersProcessed)
	}

	// Verify migration if requested
	if opts.VerifyData {
		if opts.ProgressLog {
			fmt.Printf("[%s] Verifying migration...\n", time.Now().Format("15:04:05"))
		}
		err = verifyMigration(sourceDB, pebbleDB, opts)
		if err != nil {
			return stats, fmt.Errorf("migration verification failed: %w", err)
		}
		if opts.ProgressLog {
			fmt.Printf("[%s] Migration verification passed\n", time.Now().Format("15:04:05"))
		}
	}

	return stats, nil
}

// === Helper Functions ===

// countTotalRecordsFast quickly counts total events and transfers using optimized queries
func countTotalRecordsFast(sqlitePath string) (int64, int64, error) {
	db, err := sql.Open("sqlite3", sqlitePath+"?mode=ro")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer db.Close()

	var eventCount, transferCount int64

	// Count events
	err = db.QueryRow("SELECT COUNT(*) FROM event").Scan(&eventCount)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count events: %w", err)
	}

	// Count transfers
	err = db.QueryRow("SELECT COUNT(*) FROM transfer").Scan(&transferCount)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count transfers: %w", err)
	}

	return eventCount, transferCount, nil
}

// getDirSize calculates the total size of a directory or file
func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

// verifyMigration performs basic verification that migration was successful
func verifyMigration(source, target logsdb.LogsDB, opts *MigrationOptions) error {
	ctx := context.Background()

	// Test 1: Check total event count matches
	sourceEvents, err := source.FilterEvents(ctx, &logsdb.EventFilter{
		Options: &logsdb.Options{Limit: 1000000000}, // Large limit to get count
	})
	if err != nil {
		return fmt.Errorf("failed to count source events: %w", err)
	}

	targetEvents, err := target.FilterEvents(ctx, &logsdb.EventFilter{
		Options: &logsdb.Options{Limit: 1000000000},
	})
	if err != nil {
		return fmt.Errorf("failed to count target events: %w", err)
	}

	if len(sourceEvents) != len(targetEvents) {
		return fmt.Errorf("event count mismatch: source=%d, target=%d", len(sourceEvents), len(targetEvents))
	}

	// Test 2: Check total transfer count matches
	sourceTransfers, err := source.FilterTransfers(ctx, &logsdb.TransferFilter{
		Options: &logsdb.Options{Limit: 1000000000},
	})
	if err != nil {
		return fmt.Errorf("failed to count source transfers: %w", err)
	}

	targetTransfers, err := target.FilterTransfers(ctx, &logsdb.TransferFilter{
		Options: &logsdb.Options{Limit: 1000000000},
	})
	if err != nil {
		return fmt.Errorf("failed to count target transfers: %w", err)
	}

	if len(sourceTransfers) != len(targetTransfers) {
		return fmt.Errorf("transfer count mismatch: source=%d, target=%d", len(sourceTransfers), len(targetTransfers))
	}

	return nil
}

// === Cursor-Based Migration Implementation ===

// migrateEventsCursorBased migrates events using cursor-based SQLite queries for optimal performance
func migrateEventsCursorBased(sqlitePath string, target logsdb.LogsDB, totalEvents int64, opts *MigrationOptions) (int64, error) {
	// Open SQLite database directly for cursor-based queries
	db, err := sql.Open("sqlite3", sqlitePath+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer db.Close()

	// Prepare cursor-based query (eliminates OFFSET performance degradation)
	query := `
		SELECT seq, address, topic0, topic1, topic2, topic3, topic4, data 
		FROM event 
		WHERE seq > ? 
		ORDER BY seq ASC 
		LIMIT ?`

	stmt, err := db.Prepare(query)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare event query: %w", err)
	}
	defer stmt.Close()

	writer := target.NewWriter()
	defer writer.Rollback()

	var processedEvents int64
	lastSeq := int64(0)
	batchCount := 0

	for processedEvents < totalEvents {
		// Execute cursor-based query starting from lastSeq
		rows, err := stmt.Query(lastSeq, opts.BatchSize)
		if err != nil {
			return processedEvents, fmt.Errorf("failed to execute event query: %w", err)
		}

		events := make([]*logsdb.Event, 0, opts.BatchSize)

		// Process this batch
		for rows.Next() {
			var seq int64
			var addressBytes, topic0Bytes, topic1Bytes, topic2Bytes, topic3Bytes, topic4Bytes, data []byte

			err := rows.Scan(&seq, &addressBytes, &topic0Bytes, &topic1Bytes, &topic2Bytes, &topic3Bytes, &topic4Bytes, &data)
			if err != nil {
				rows.Close()
				return processedEvents, fmt.Errorf("failed to scan event row: %w", err)
			}

			// Convert SQLite sequence to block/tx/log indices
			seqValue := sequence(seq)
			blockNumber := seqValue.BlockNumber()
			txIndex := seqValue.TxIndex()
			logIndex := seqValue.LogIndex()

			// Parse address and topics
			address := thor.Address{}
			copy(address[:], addressBytes)

			var topics [5]*thor.Bytes32
			topicBytesList := [][]byte{topic0Bytes, topic1Bytes, topic2Bytes, topic3Bytes, topic4Bytes}
			for i, topicBytes := range topicBytesList {
				if len(topicBytes) == 32 {
					var topic thor.Bytes32
					copy(topic[:], topicBytes)
					topics[i] = &topic
				}
			}

			event := &logsdb.Event{
				Address:     address,
				Topics:      topics,
				Data:        data,
				BlockNumber: blockNumber,
				TxIndex:     txIndex,
				LogIndex:    logIndex,
			}

			events = append(events, event)
			lastSeq = seq
		}
		rows.Close()

		if len(events) == 0 {
			break // No more events
		}

		// Write batch to target
		err = migrateEventBatch(events, writer, opts)
		if err != nil {
			return processedEvents, fmt.Errorf("failed to write event batch: %w", err)
		}

		processedEvents += int64(len(events))
		batchCount++

		// Progress logging
		if opts.ProgressLog && batchCount%10 == 0 {
			progress := float64(processedEvents) / float64(totalEvents) * 100
			fmt.Printf("[%s] Events: %d/%d (%.1f%%)\n",
				time.Now().Format("15:04:05"), processedEvents, totalEvents, progress)
		}

		// Commit every few batches for better performance
		if batchCount%5 == 0 {
			if pebbleWriter, ok := writer.(*pebbledb.PebbleV3Writer); ok {
				if err := pebbleWriter.CommitNoSync(); err != nil {
					writer.Rollback()
					return processedEvents, fmt.Errorf("failed to commit event batch: %w", err)
				}
			} else {
				if err := writer.Commit(); err != nil {
					writer.Rollback()
					return processedEvents, fmt.Errorf("failed to commit event batch: %w", err)
				}
			}
		}
	}

	// Final commit
	if pebbleWriter, ok := writer.(*pebbledb.PebbleV3Writer); ok {
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

// migrateTransfersCursorBased migrates transfers using cursor-based SQLite queries
func migrateTransfersCursorBased(sqlitePath string, target logsdb.LogsDB, totalTransfers int64, opts *MigrationOptions) (int64, error) {
	// Open SQLite database directly for cursor-based queries
	db, err := sql.Open("sqlite3", sqlitePath+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer db.Close()

	query := `
		SELECT seq, sender, recipient, amount 
		FROM transfer 
		WHERE seq > ? 
		ORDER BY seq ASC 
		LIMIT ?`

	stmt, err := db.Prepare(query)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare transfer query: %w", err)
	}
	defer stmt.Close()

	writer := target.NewWriter()
	defer writer.Rollback()

	var processedTransfers int64
	lastSeq := int64(0)
	batchCount := 0

	for processedTransfers < totalTransfers {
		rows, err := stmt.Query(lastSeq, opts.BatchSize)
		if err != nil {
			return processedTransfers, fmt.Errorf("failed to execute transfer query: %w", err)
		}

		transfers := make([]*logsdb.Transfer, 0, opts.BatchSize)

		for rows.Next() {
			var seq int64
			var senderBytes, recipientBytes, amountBytes []byte

			err := rows.Scan(&seq, &senderBytes, &recipientBytes, &amountBytes)
			if err != nil {
				rows.Close()
				return processedTransfers, fmt.Errorf("failed to scan transfer row: %w", err)
			}

			seqValue := sequence(seq)
			blockNumber := seqValue.BlockNumber()
			txIndex := seqValue.TxIndex()
			logIndex := seqValue.LogIndex()

			sender := thor.Address{}
			copy(sender[:], senderBytes)

			recipient := thor.Address{}
			copy(recipient[:], recipientBytes)

			amount := new(big.Int)
			amount.SetBytes(amountBytes)

			transfer := &logsdb.Transfer{
				Sender:      sender,
				Recipient:   recipient,
				Amount:      amount,
				BlockNumber: blockNumber,
				TxIndex:     txIndex,
				LogIndex:    logIndex,
			}

			transfers = append(transfers, transfer)
			lastSeq = seq
		}
		rows.Close()

		if len(transfers) == 0 {
			break
		}

		err = migrateTransferBatch(transfers, writer, opts)
		if err != nil {
			return processedTransfers, fmt.Errorf("failed to write transfer batch: %w", err)
		}

		processedTransfers += int64(len(transfers))
		batchCount++

		if opts.ProgressLog && batchCount%10 == 0 {
			progress := float64(processedTransfers) / float64(totalTransfers) * 100
			fmt.Printf("[%s] Transfers: %d/%d (%.1f%%)\n",
				time.Now().Format("15:04:05"), processedTransfers, totalTransfers, progress)
		}

		if batchCount%5 == 0 {
			if pebbleWriter, ok := writer.(*pebbledb.PebbleV3Writer); ok {
				if err := pebbleWriter.CommitNoSync(); err != nil {
					writer.Rollback()
					return processedTransfers, fmt.Errorf("failed to commit transfer batch: %w", err)
				}
			} else {
				if err := writer.Commit(); err != nil {
					writer.Rollback()
					return processedTransfers, fmt.Errorf("failed to commit transfer batch: %w", err)
				}
			}
		}
	}

	// Final commit
	if pebbleWriter, ok := writer.(*pebbledb.PebbleV3Writer); ok {
		if err := pebbleWriter.CommitNoSync(); err != nil {
			return processedTransfers, fmt.Errorf("failed to commit final transfer batch: %w", err)
		}
	} else {
		if err := writer.Commit(); err != nil {
			return processedTransfers, fmt.Errorf("failed to commit final transfer batch: %w", err)
		}
	}

	return processedTransfers, nil
}

// migrateEventBatch writes a batch of events to the target database
// Events are grouped by block number and written as synthetic blocks
func migrateEventBatch(events []*logsdb.Event, writer logsdb.Writer, opts *MigrationOptions) error {
	// Group events by block number
	eventsByBlock := make(map[uint32][]*logsdb.Event)
	for _, event := range events {
		eventsByBlock[event.BlockNumber] = append(eventsByBlock[event.BlockNumber], event)
	}

	// Write each block's events
	for blockNum, blockEvents := range eventsByBlock {
		// Create a synthetic block
		block := createSyntheticBlock(blockNum)

		// Create synthetic receipts from events
		receipts := createReceiptsFromEvents(blockEvents)

		if err := writer.Write(block, receipts); err != nil {
			return fmt.Errorf("failed to write events for block %d: %w", blockNum, err)
		}
	}
	return nil
}

// migrateTransferBatch writes a batch of transfers to the target database
// Transfers are grouped by block number and written as synthetic blocks
func migrateTransferBatch(transfers []*logsdb.Transfer, writer logsdb.Writer, opts *MigrationOptions) error {
	// Group transfers by block number
	transfersByBlock := make(map[uint32][]*logsdb.Transfer)
	for _, transfer := range transfers {
		transfersByBlock[transfer.BlockNumber] = append(transfersByBlock[transfer.BlockNumber], transfer)
	}

	// Write each block's transfers
	for blockNum, blockTransfers := range transfersByBlock {
		// Create a synthetic block
		block := createSyntheticBlock(blockNum)

		// Create synthetic receipts from transfers
		receipts := createReceiptsFromTransfers(blockTransfers)

		if err := writer.Write(block, receipts); err != nil {
			return fmt.Errorf("failed to write transfers for block %d: %w", blockNum, err)
		}
	}
	return nil
}

// === Helper Functions for Synthetic Block/Receipt Creation ===

// createSyntheticBlock creates a minimal synthetic block for the given block number
func createSyntheticBlock(blockNum uint32) *block.Block {
	// Create a synthetic parent ID based on block number
	var parentID thor.Bytes32
	if blockNum > 0 {
		// Simple derivation from previous block number
		parentID[28] = byte(blockNum - 1)
		parentID[29] = byte((blockNum - 1) >> 8)
		parentID[30] = byte((blockNum - 1) >> 16)
		parentID[31] = byte((blockNum - 1) >> 24)
	}

	return new(block.Builder).
		ParentID(parentID).
		Timestamp(uint64(1640000000 + blockNum*10)). // Synthetic timestamp
		TotalScore(uint64(blockNum)).                // Use block number as score
		GasLimit(10000000).                          // Standard gas limit
		Build()
}

// createReceiptsFromEvents converts logsdb events to transaction receipts
func createReceiptsFromEvents(events []*logsdb.Event) tx.Receipts {
	// Group events by transaction index
	eventsByTx := make(map[uint32][]*logsdb.Event)
	for _, event := range events {
		eventsByTx[event.TxIndex] = append(eventsByTx[event.TxIndex], event)
	}

	receipts := make(tx.Receipts, 0, len(eventsByTx))

	for txIndex := uint32(0); txIndex <= getMaxTxIndex(events); txIndex++ {
		txEvents, exists := eventsByTx[txIndex]
		if !exists {
			continue
		}

		// Convert logsdb.Events to tx.Events
		txEventsList := make([]*tx.Event, 0, len(txEvents))
		for _, event := range txEvents {
			txEvent := &tx.Event{
				Address: event.Address,
				Topics:  convertTopicsToSlice(event.Topics),
				Data:    event.Data,
			}
			txEventsList = append(txEventsList, txEvent)
		}

		// Create receipt with events
		receipt := &tx.Receipt{
			Outputs: []*tx.Output{
				{Events: txEventsList},
			},
		}
		receipts = append(receipts, receipt)
	}

	return receipts
}

// createReceiptsFromTransfers converts logsdb transfers to transaction receipts
func createReceiptsFromTransfers(transfers []*logsdb.Transfer) tx.Receipts {
	// Group transfers by transaction index
	transfersByTx := make(map[uint32][]*logsdb.Transfer)
	for _, transfer := range transfers {
		transfersByTx[transfer.TxIndex] = append(transfersByTx[transfer.TxIndex], transfer)
	}

	receipts := make(tx.Receipts, 0, len(transfersByTx))

	for txIndex := uint32(0); txIndex <= getMaxTxIndexTransfers(transfers); txIndex++ {
		txTransfers, exists := transfersByTx[txIndex]
		if !exists {
			continue
		}

		// Convert logsdb.Transfers to tx.Transfers
		txTransfersList := make([]*tx.Transfer, 0, len(txTransfers))
		for _, transfer := range txTransfers {
			txTransfer := &tx.Transfer{
				Sender:    transfer.Sender,
				Recipient: transfer.Recipient,
				Amount:    transfer.Amount,
			}
			txTransfersList = append(txTransfersList, txTransfer)
		}

		// Create receipt with transfers
		receipt := &tx.Receipt{
			Outputs: []*tx.Output{
				{Transfers: txTransfersList},
			},
		}
		receipts = append(receipts, receipt)
	}

	return receipts
}

// getMaxTxIndex finds the maximum transaction index in events
func getMaxTxIndex(events []*logsdb.Event) uint32 {
	var maxTxIndex uint32
	for _, event := range events {
		if event.TxIndex > maxTxIndex {
			maxTxIndex = event.TxIndex
		}
	}
	return maxTxIndex
}

// getMaxTxIndexTransfers finds the maximum transaction index in transfers
func getMaxTxIndexTransfers(transfers []*logsdb.Transfer) uint32 {
	var maxTxIndex uint32
	for _, transfer := range transfers {
		if transfer.TxIndex > maxTxIndex {
			maxTxIndex = transfer.TxIndex
		}
	}
	return maxTxIndex
}

// convertTopicsToSlice converts [5]*thor.Bytes32 to []thor.Bytes32
func convertTopicsToSlice(topics [5]*thor.Bytes32) []thor.Bytes32 {
	result := make([]thor.Bytes32, 0, 5)
	for _, topic := range topics {
		if topic != nil {
			result = append(result, *topic)
		}
	}
	return result
}
