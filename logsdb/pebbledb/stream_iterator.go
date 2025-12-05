// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"bytes"
	"log"

	"github.com/cockroachdb/pebble"
)

// StreamIterator provides streaming access to sequences with bounds-only approach
type StreamIterator struct {
	iter            *pebble.Iterator
	exhausted       bool
	filterIndexKeys bool // Only filter index keys for primary range iterators
	keyPrefix       byte // Expected key prefix (0x45 for events, 0x54 for transfers) when filtering

	// New fields for fast-seek optimization
	ascending  bool
	minSeq     sequence
	maxSeq     sequence
	lowerBound []byte // inclusive
	upperBound []byte // exclusive
}

// NewExhaustedStreamIterator creates a safely exhausted iterator
func NewExhaustedStreamIterator() *StreamIterator {
	return &StreamIterator{
		iter:       nil,
		exhausted:  true,
		ascending:  true,
		minSeq:     0,
		maxSeq:     0,
		lowerBound: nil,
		upperBound: nil,
		// filterIndexKeys/keyPrefix irrelevant when exhausted
	}
}

// NewStreamIterator creates a new StreamIterator with the given Pebble iterator
// All range and order constraints are enforced via LowerBound/UpperBound and initial Seek
func NewStreamIterator(
	iter *pebble.Iterator,
	minSeq, maxSeq sequence,
	ascending bool,
	lowerBound, upperBound []byte,
) *StreamIterator {
	return &StreamIterator{
		iter:            iter,
		exhausted:       !iter.Valid(),
		filterIndexKeys: false,
		ascending:       ascending,
		minSeq:          minSeq,
		maxSeq:          maxSeq,
		lowerBound:      lowerBound,
		upperBound:      upperBound,
	}
}

// NewEventPrimaryStreamIterator creates a StreamIterator for event primary range queries
// This iterator will filter out everything except E<seq> keys
func NewEventPrimaryStreamIterator(
	iter *pebble.Iterator,
	minSeq, maxSeq sequence,
	ascending bool,
	lowerBound, upperBound []byte,
) *StreamIterator {
	return &StreamIterator{
		iter:            iter,
		exhausted:       !iter.Valid(),
		filterIndexKeys: true,
		keyPrefix:       0x45, // Only accept "E" keys
		ascending:       ascending,
		minSeq:          minSeq,
		maxSeq:          maxSeq,
		lowerBound:      lowerBound,
		upperBound:      upperBound,
	}
}

// NewTransferPrimaryStreamIterator creates a StreamIterator for transfer primary range queries
// This iterator will filter out everything except T<seq> keys
func NewTransferPrimaryStreamIterator(
	iter *pebble.Iterator,
	minSeq, maxSeq sequence,
	ascending bool,
	lowerBound, upperBound []byte,
) *StreamIterator {
	return &StreamIterator{
		iter:            iter,
		exhausted:       !iter.Valid(),
		filterIndexKeys: true,
		keyPrefix:       0x54, // Only accept "T" keys
		ascending:       ascending,
		minSeq:          minSeq,
		maxSeq:          maxSeq,
		lowerBound:      lowerBound,
		upperBound:      upperBound,
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

	// 1. If already exhausted, return immediately
	if s.exhausted {
		return 0, false
	}

	// 2. If iter is nil or invalid, mark exhausted and return
	if s.iter == nil || !s.iter.Valid() {
		s.exhausted = true
		s.checkIteratorError()
		return 0, false
	}

	for {
		// 3. Now safe to access s.iter.Key(), bounds, etc
		currentKey := s.iter.Key()

		// 4. Check bounds first - lexicographic comparison
		if s.ascending {
			if bytes.Compare(currentKey, s.upperBound) >= 0 {
				s.exhausted = true
				s.checkIteratorError()
				return 0, false
			}
		} else {
			if bytes.Compare(currentKey, s.lowerBound) < 0 {
				s.exhausted = true
				s.checkIteratorError()
				return 0, false
			}
		}

		// 5. Apply prefix filtering if enabled (primary iterators only)
		if s.filterIndexKeys && !s.isValidKeyForPrefix(currentKey) {
			// Advance iterator based on direction
			if s.ascending {
				s.iter.Next()
			} else {
				s.iter.Prev()
			}

			// Check validity after movement
			if s.iter == nil || !s.iter.Valid() {
				s.exhausted = true
				s.checkIteratorError()
				return 0, false
			}
			continue
		}

		// 6. Valid key - extract sequence and advance for next call
		seq := sequenceFromKey(currentKey)

		// Advance for next call
		if s.ascending {
			s.iter.Next()
		} else {
			s.iter.Prev()
		}

		// Update exhaustion state for next call
		if s.iter == nil || !s.iter.Valid() {
			s.exhausted = true
		}

		return seq, true
	}
}

// Helper method for error logging
func (s *StreamIterator) checkIteratorError() {
	if s.iter != nil {
		if err := s.iter.Error(); err != nil {
			// Log at debug level if logging infrastructure exists
			log.Printf("DEBUG: StreamIterator error: %v", err)
		}
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
