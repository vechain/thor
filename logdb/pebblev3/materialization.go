// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebblev3

import (
	"github.com/cockroachdb/pebble"
	"github.com/vechain/thor/v2/logdb"
)

// materializeEvents loads actual event records from primary storage
func (q *StreamingQueryEngine) materializeEvents(sequences []sequence) ([]*logdb.Event, error) {
	results := make([]*logdb.Event, 0, len(sequences))
	
	for _, seq := range sequences {
		// Primary key lookup: E/<seq>
		eventKey := eventPrimaryKey(seq)
		value, closer, err := q.db.Get(eventKey)
		if err != nil {
			if err == pebble.ErrNotFound {
				continue // Skip missing records
			}
			return nil, err
		}
		
		// Decode RLP data
		var eventRecord EventRecord
		if err := eventRecord.RLPDecode(value); err != nil {
			closer.Close()
			continue // Skip corrupted records
		}
		closer.Close()
		
		// Convert to LogDB format
		event := eventRecord.ToLogDBEvent()
		results = append(results, event)
	}
	
	return results, nil
}

// materializeTransfers loads actual transfer records from primary storage
func (q *StreamingQueryEngine) materializeTransfers(sequences []sequence) ([]*logdb.Transfer, error) {
	results := make([]*logdb.Transfer, 0, len(sequences))
	
	for _, seq := range sequences {
		// Primary key lookup: T/<seq>
		transferKey := transferPrimaryKey(seq)
		value, closer, err := q.db.Get(transferKey)
		if err != nil {
			if err == pebble.ErrNotFound {
				continue // Skip missing records
			}
			return nil, err
		}
		
		// Decode RLP data
		var transferRecord TransferRecord
		if err := transferRecord.RLPDecode(value); err != nil {
			closer.Close()
			continue // Skip corrupted records
		}
		closer.Close()
		
		// Convert to LogDB format
		transfer := transferRecord.ToLogDBTransfer()
		results = append(results, transfer)
	}
	
	return results, nil
}