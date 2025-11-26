// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebblev3

import (
	"github.com/cockroachdb/pebble"
)

// StreamIterator provides streaming access to sequences with bounds-only approach
type StreamIterator struct {
	iter      *pebble.Iterator
	exhausted bool
}

// NewStreamIterator creates a new StreamIterator with the given Pebble iterator
// All range and order constraints are enforced via LowerBound/UpperBound and initial Seek
func NewStreamIterator(iter *pebble.Iterator, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	return &StreamIterator{
		iter:      iter,
		exhausted: !iter.Valid(),
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
	
	// Get current sequence
	seq := sequenceFromKey(s.iter.Key())
	
	// Advance iterator
	s.iter.Next()
	if !s.iter.Valid() {
		s.exhausted = true
	}
	
	return seq, true
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