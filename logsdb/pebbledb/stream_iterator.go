// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"github.com/cockroachdb/pebble"
)

// StreamIterator provides streaming access to sequences with bounds-only approach
type StreamIterator struct {
	iter            *pebble.Iterator
	exhausted       bool
	filterIndexKeys bool // Only filter index keys for primary range iterators
	keyPrefix       byte // Expected key prefix (0x45 for events, 0x54 for transfers) when filtering
}

// NewStreamIterator creates a new StreamIterator with the given Pebble iterator
// All range and order constraints are enforced via LowerBound/UpperBound and initial Seek
func NewStreamIterator(iter *pebble.Iterator, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	return &StreamIterator{
		iter:            iter,
		exhausted:       !iter.Valid(),
		filterIndexKeys: false, // By default, don't filter index keys
	}
}

// NewEventPrimaryStreamIterator creates a StreamIterator for event primary range queries
// This iterator will filter out everything except E<seq> keys
func NewEventPrimaryStreamIterator(iter *pebble.Iterator, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	return &StreamIterator{
		iter:            iter,
		exhausted:       !iter.Valid(),
		filterIndexKeys: true, // Filter keys for primary range queries
		keyPrefix:       0x45, // Only accept "E" keys
	}
}

// NewTransferPrimaryStreamIterator creates a StreamIterator for transfer primary range queries
// This iterator will filter out everything except T<seq> keys
func NewTransferPrimaryStreamIterator(iter *pebble.Iterator, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	return &StreamIterator{
		iter:            iter,
		exhausted:       !iter.Valid(),
		filterIndexKeys: true, // Filter keys for primary range queries
		keyPrefix:       0x54, // Only accept "T" keys
	}
}

// Current returns the current sequence without advancing the iterator
func (s *StreamIterator) Current() (sequence, bool) {
	if s.exhausted || !s.iter.Valid() {
		return 0, false
	}
	seq := sequenceFromKey(s.iter.Key())
	return seq, true
}

// Next advances the iterator and returns the next sequence
func (s *StreamIterator) Next() (sequence, bool) {
	recordNext() // Debug metrics

	if s.exhausted || !s.iter.Valid() {
		return 0, false
	}

	// Use a loop instead of recursion to avoid stack overflow with many index keys
	for {
		// Get current sequence
		currentKey := s.iter.Key()

		// Only filter keys if this iterator is configured for primary range queries
		if s.filterIndexKeys && !s.isValidKeyForPrefix(currentKey) {
			// This key doesn't match our expected prefix, skip it and try the next key
			s.iter.Next()
			if !s.iter.Valid() {
				s.exhausted = true
				return 0, false
			}
			// Continue loop to try the next key
			continue
		}

		// Found a valid key
		seq := sequenceFromKey(currentKey)

		// Advance iterator for next call
		s.iter.Next()
		if !s.iter.Valid() {
			s.exhausted = true
		}

		return seq, true
	}
}

// Close releases the underlying Pebble iterator
func (s *StreamIterator) Close() error {
	if s.iter != nil {
		return s.iter.Close()
	}
	return nil
}

// IsExhausted returns true if the iterator has no more values
func (s *StreamIterator) IsExhausted() bool {
	return s.exhausted || !s.iter.Valid()
}

// isValidKeyForPrefix checks if a key matches the expected prefix and format for this iterator
func (s *StreamIterator) isValidKeyForPrefix(key []byte) bool {
	if len(key) != 9 {
		return false // Primary keys are exactly prefix + 8-byte sequence = 9 bytes
	}

	return key[0] == s.keyPrefix
}
