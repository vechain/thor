// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"sync"

	"github.com/cockroachdb/pebble"

	"github.com/vechain/thor/v2/logsdb"
)

// Object pools for reducing allocations during materialization
var (
	eventRecordPool = sync.Pool{
		New: func() interface{} {
			return &EventRecord{}
		},
	}
	
	transferRecordPool = sync.Pool{
		New: func() interface{} {
			return &TransferRecord{}
		},
	}
)

// materializeEvents loads actual event records from primary storage
func (q *StreamingQueryEngine) materializeEvents(sequences []sequence) ([]*logsdb.Event, error) {
	results := make([]*logsdb.Event, 0, len(sequences))

	for _, seq := range sequences {
		// Primary key lookup: E/<seq> - use reusable buffer
		eventKey := q.buildEventPrimaryKey(seq)
		value, closer, err := q.db.Get(eventKey)
		if err != nil {
			if err == pebble.ErrNotFound {
				continue // Skip missing records
			}
			return nil, err
		}

		// Get pooled EventRecord for decoding
		eventRecord := eventRecordPool.Get().(*EventRecord)
		// Reset the record to avoid stale data from previous use
		eventRecord.reset()
		
		if err := eventRecord.Decode(value); err != nil {
			closer.Close()
			eventRecordPool.Put(eventRecord) // Return to pool even on error
			continue // Skip corrupted records
		}
		closer.Close()

		// Convert to LogDB format (this still allocates a new logsdb.Event for safe return)
		event := eventRecord.ToLogDBEvent()
		
		// Return EventRecord to pool for reuse
		eventRecordPool.Put(eventRecord)
		
		results = append(results, event)
	}

	return results, nil
}

// materializeTransfers loads actual transfer records from primary storage
func (q *StreamingQueryEngine) materializeTransfers(sequences []sequence) ([]*logsdb.Transfer, error) {
	results := make([]*logsdb.Transfer, 0, len(sequences))

	for _, seq := range sequences {
		// Primary key lookup: T/<seq> - use reusable buffer
		transferKey := q.buildTransferPrimaryKey(seq)
		value, closer, err := q.db.Get(transferKey)
		if err != nil {
			if err == pebble.ErrNotFound {
				continue // Skip missing records
			}
			return nil, err
		}

		// Get pooled TransferRecord for decoding
		transferRecord := transferRecordPool.Get().(*TransferRecord)
		// Reset the record to avoid stale data from previous use
		transferRecord.reset()
		
		if err := transferRecord.Decode(value); err != nil {
			closer.Close()
			transferRecordPool.Put(transferRecord) // Return to pool even on error
			continue // Skip corrupted records
		}
		closer.Close()

		// Convert to LogDB format (this still allocates a new logsdb.Transfer for safe return)
		transfer := transferRecord.ToLogDBTransfer()
		
		// Return TransferRecord to pool for reuse
		transferRecordPool.Put(transferRecord)
		
		results = append(results, transfer)
	}

	return results, nil
}
