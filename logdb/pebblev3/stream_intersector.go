// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebblev3

import (
	"log"
)

// StreamIntersector implements AND logic within a single criterion using leapfrog intersection
type StreamIntersector struct {
	streams   []*StreamIterator
	ascending bool
}

// NewStreamIntersector creates a new StreamIntersector with the given streams
func NewStreamIntersector(streams []*StreamIterator, ascending bool) *StreamIntersector {
	return &StreamIntersector{
		streams:   streams,
		ascending: ascending,
	}
}

// Next returns the next sequence that appears in ALL streams (AND logic)
// Strategy: Always compute intersection in ASC order regardless of query direction
func (s *StreamIntersector) Next() (sequence, bool) {
	if len(s.streams) == 0 {
		return 0, false
	}
	
	// Single stream optimization
	if len(s.streams) == 1 {
		return s.streams[0].Next()
	}
	
	// Leapfrog intersection algorithm (always in ASC order)
	for {
		var target sequence
		allMatch := true
		anyExhausted := false
		
		// Find maximum sequence among current positions
		for i, stream := range s.streams {
			if seq, hasValue := stream.Current(); hasValue {
				if i == 0 || seq > target {
					target = seq
				}
			} else {
				anyExhausted = true
				break
			}
		}
		
		if anyExhausted {
			return 0, false
		}
		
		// Advance all streams to at least target
		for _, stream := range s.streams {
			// Advance stream until it reaches or exceeds target
			for {
				currentSeq, hasValue := stream.Current()
				if !hasValue {
					return 0, false // Stream exhausted
				}
				if currentSeq >= target {
					break // This stream is now at or past target
				}
				// Need to advance
				if _, hasNext := stream.Next(); !hasNext {
					return 0, false // Stream exhausted while advancing
				}
			}
			
			// Check if this stream is now at target
			currentSeq, hasValue := stream.Current()
			if !hasValue || currentSeq != target {
				allMatch = false
				break
			}
		}
		
		if allMatch {
			// All streams are at the same sequence - this is our intersection
			result := target
			
			// Advance all streams past this result for next iteration
			for _, stream := range s.streams {
				stream.Next()
			}
			
			return result, true
		}
		
		// Not all streams matched - continue the leapfrog process
		// The loop will continue with the new maximum sequence
	}
}

// Close releases all underlying stream iterators
func (s *StreamIntersector) Close() error {
	var lastErr error
	for _, stream := range s.streams {
		if err := stream.Close(); err != nil {
			log.Printf("Error closing stream iterator: %v", err)
			lastErr = err
		}
	}
	return lastErr
}

// IsExhausted returns true if any underlying stream is exhausted
func (s *StreamIntersector) IsExhausted() bool {
	for _, stream := range s.streams {
		if stream.IsExhausted() {
			return true
		}
	}
	return false
}