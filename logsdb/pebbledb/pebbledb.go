// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"context"
	"errors"
	"log"

	"github.com/cockroachdb/pebble"

	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
)

var _ logsdb.LogsDB = (*PebbleDBLogDB)(nil)

// PebbleDBLogDB implements logsdb.LogDB interface using PebbleDB v3 streaming architecture
type PebbleDBLogDB struct {
	db          *pebble.DB
	path        string
	queryEngine *StreamingQueryEngine
}

// ErrorOnlyLogger implements pebble.Logger to reduce noise
type ErrorOnlyLogger struct{}

func (l ErrorOnlyLogger) Infof(format string, args ...interface{})  {}
func (l ErrorOnlyLogger) Warnf(format string, args ...interface{})  {}
func (l ErrorOnlyLogger) Fatalf(format string, args ...interface{}) {}
func (l ErrorOnlyLogger) Errorf(format string, args ...interface{}) {
	log.Printf("PEBBLE ERROR: "+format, args...)
}

// Open opens the PebbleDB v3 database at the given path
func Open(path string) (*PebbleDBLogDB, error) {
	return openWithOptions(path, defaultOptions())
}

// OpenForBulkLoad opens the PebbleDB v3 database optimized for bulk loading/migration
func OpenForBulkLoad(path string) (*PebbleDBLogDB, error) {
	return openWithOptions(path, bulkLoadOptions())
}

// defaultOptions returns standard options for normal operation
func defaultOptions() *pebble.Options {
	return &pebble.Options{
		// Optimize for log data - LSM tree is good for sequential writes
		L0CompactionThreshold:       4,
		L0StopWritesThreshold:       12,
		LBaseMaxBytes:               64 << 20, // 64MB
		MaxOpenFiles:                1000,
		MemTableSize:                32 << 20, // 32MB
		MemTableStopWritesThreshold: 4,
		Logger:                      ErrorOnlyLogger{},
	}
}

// bulkLoadOptions returns options optimized for bulk loading during migration
func bulkLoadOptions() *pebble.Options {
	return &pebble.Options{
		// Bulk loading optimizations
		L0CompactionThreshold:       16,        // Allow more L0 files before compaction
		L0StopWritesThreshold:       32,        // Higher threshold before stopping writes
		LBaseMaxBytes:               256 << 20, // 256MB - larger base level
		MaxOpenFiles:                2000,      // More open files for bulk operations
		MemTableSize:                128 << 20, // 128MB - much larger memtable
		MemTableStopWritesThreshold: 8,         // More memtables before stopping
		DisableWAL:                  true,      // Disable WAL for maximum speed (less safe)
		Logger:                      ErrorOnlyLogger{},
	}
}

// openWithOptions opens the database with the given options
func openWithOptions(path string, opts *pebble.Options) (*PebbleDBLogDB, error) {
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, err
	}

	return &PebbleDBLogDB{
		db:          db,
		path:        path,
		queryEngine: NewStreamingQueryEngine(db),
	}, nil
}

// Close closes the database
func (p *PebbleDBLogDB) Close() error {
	return p.db.Close()
}

// NewWriter creates a new writer instance
func (p *PebbleDBLogDB) NewWriter() logsdb.Writer {
	return NewPebbleDBWriter(p.db)
}

// NewBulkWriter creates a writer optimized for bulk loading/migration
func (p *PebbleDBLogDB) NewBulkWriter() logsdb.Writer {
	// For bulk loading, we use the same writer but it can be optimized differently
	return NewPebbleDBWriter(p.db)
}

// NewWriterSyncOff creates a new writer instance (same as NewWriter for PebbleDB)
func (p *PebbleDBLogDB) NewWriterSyncOff() logsdb.Writer {
	return p.NewWriter()
}

// Path returns the database path
func (p *PebbleDBLogDB) Path() string {
	return p.path
}

// NewestBlockID returns the newest written block ID
func (p *PebbleDBLogDB) NewestBlockID() (thor.Bytes32, error) {
	// Iterate in reverse order over event primary keys to find the latest
	opts := &pebble.IterOptions{
		LowerBound: []byte(eventPrimaryPrefix),
		UpperBound: []byte(eventPrimaryPrefix + "\xff"),
	}

	iter, err := p.db.NewIter(opts)
	if err != nil {
		return thor.Bytes32{}, err
	}
	defer iter.Close()

	// Seek to last key
	if iter.Last() {
		seq := sequenceFromKey(iter.Key())
		if seq == 0 {
			return thor.Bytes32{}, errors.New("invalid sequence in newest block lookup")
		}

		// Get the event record to extract BlockID
		value, closer, err := p.db.Get(iter.Key())
		if err != nil {
			return thor.Bytes32{}, err
		}
		defer closer.Close()

		var eventRecord EventRecord
		if err := eventRecord.RLPDecode(value); err != nil {
			return thor.Bytes32{}, err
		}

		return eventRecord.BlockID, nil
	}

	// No events found - check transfers
	transferOpts := &pebble.IterOptions{
		LowerBound: []byte(transferPrimaryPrefix),
		UpperBound: []byte(transferPrimaryPrefix + "\xff"),
	}

	transferIter, err := p.db.NewIter(transferOpts)
	if err != nil {
		return thor.Bytes32{}, err
	}
	defer transferIter.Close()

	if transferIter.Last() {
		seq := sequenceFromKey(transferIter.Key())
		if seq == 0 {
			return thor.Bytes32{}, errors.New("invalid sequence in newest block lookup")
		}

		// Get the transfer record to extract BlockID
		value, closer, err := p.db.Get(transferIter.Key())
		if err != nil {
			return thor.Bytes32{}, err
		}
		defer closer.Close()

		var transferRecord TransferRecord
		if err := transferRecord.RLPDecode(value); err != nil {
			return thor.Bytes32{}, err
		}

		return transferRecord.BlockID, nil
	}

	return thor.Bytes32{}, nil
}

// HasBlockID checks if a block ID exists in the database
func (p *PebbleDBLogDB) HasBlockID(blockID thor.Bytes32) (bool, error) {
	// Strategy: Check both events and transfers for any record with this BlockID
	// This is not optimized but is correct - in practice, could add BlockID index

	// Check events first
	eventOpts := &pebble.IterOptions{
		LowerBound: []byte(eventPrimaryPrefix),
		UpperBound: []byte(eventPrimaryPrefix + "\xff"),
	}

	eventIter, err := p.db.NewIter(eventOpts)
	if err != nil {
		return false, err
	}
	defer eventIter.Close()

	for eventIter.First(); eventIter.Valid(); eventIter.Next() {
		value, closer, err := p.db.Get(eventIter.Key())
		if err != nil {
			if err == pebble.ErrNotFound {
				continue
			}
			return false, err
		}

		var eventRecord EventRecord
		decodeErr := eventRecord.RLPDecode(value)
		closer.Close()

		if decodeErr == nil && eventRecord.BlockID == blockID {
			return true, nil
		}
	}

	// Check transfers
	transferOpts := &pebble.IterOptions{
		LowerBound: []byte(transferPrimaryPrefix),
		UpperBound: []byte(transferPrimaryPrefix + "\xff"),
	}

	transferIter, err := p.db.NewIter(transferOpts)
	if err != nil {
		return false, err
	}
	defer transferIter.Close()

	for transferIter.First(); transferIter.Valid(); transferIter.Next() {
		value, closer, err := p.db.Get(transferIter.Key())
		if err != nil {
			if err == pebble.ErrNotFound {
				continue
			}
			return false, err
		}

		var transferRecord TransferRecord
		decodeErr := transferRecord.RLPDecode(value)
		closer.Close()

		if decodeErr == nil && transferRecord.BlockID == blockID {
			return true, nil
		}
	}

	return false, nil
}

// FilterEvents filters events according to the given criteria using streaming query engine
func (p *PebbleDBLogDB) FilterEvents(ctx context.Context, filter *logsdb.EventFilter) ([]*logsdb.Event, error) {
	if filter == nil {
		return nil, errors.New("filter is required")
	}

	// Validate filter options
	if filter.Options == nil {
		return nil, errors.New("filter options are required")
	}

	return p.queryEngine.FilterEvents(ctx, filter)
}

// FilterTransfers filters transfers according to the given criteria using streaming query engine
func (p *PebbleDBLogDB) FilterTransfers(ctx context.Context, filter *logsdb.TransferFilter) ([]*logsdb.Transfer, error) {
	if filter == nil {
		return nil, errors.New("filter is required")
	}

	// Validate filter options
	if filter.Options == nil {
		return nil, errors.New("filter options are required")
	}

	return p.queryEngine.FilterTransfers(ctx, filter)
}
