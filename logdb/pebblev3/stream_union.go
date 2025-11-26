// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebblev3

import (
	"container/heap"
	"log"
)

// HeapItem represents an item in the priority queue for k-way merge
type HeapItem struct {
	seq         sequence
	streamIndex int
	stream      *StreamIntersector
}

// SequenceHeap implements heap.Interface for sequence-based priority queue
type SequenceHeap struct {
	items     []HeapItem
	ascending bool
}

// NewSequenceHeap creates a new heap for the given order
func NewSequenceHeap(ascending bool) *SequenceHeap {
	return &SequenceHeap{
		items:     make([]HeapItem, 0),
		ascending: ascending,
	}
}

// Len implements heap.Interface
func (h *SequenceHeap) Len() int {
	return len(h.items)
}

// Less implements heap.Interface
func (h *SequenceHeap) Less(i, j int) bool {
	if h.ascending {
		return h.items[i].seq < h.items[j].seq
	}
	return h.items[i].seq > h.items[j].seq
}

// Swap implements heap.Interface
func (h *SequenceHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

// Push implements heap.Interface
func (h *SequenceHeap) Push(x interface{}) {
	h.items = append(h.items, x.(HeapItem))
}

// Pop implements heap.Interface
func (h *SequenceHeap) Pop() interface{} {
	old := h.items
	n := len(old)
	item := old[n-1]
	h.items = old[0 : n-1]
	return item
}

// StreamUnion implements OR logic across multiple criteria using k-way merge with deduplication
type StreamUnion struct {
	heap      *SequenceHeap
	lastSeq   sequence
	ascending bool
	streams   []*StreamIntersector
}

// NewStreamUnion creates a new StreamUnion with the given criterion streams
func NewStreamUnion(criterionStreams []*StreamIntersector, ascending bool) *StreamUnion {
	sequenceHeap := NewSequenceHeap(ascending)
	
	// Prime the heap with first element from each criterion stream
	for i, stream := range criterionStreams {
		if seq, hasNext := stream.Next(); hasNext {
			recordHeapOp() // Debug metrics
			heap.Push(sequenceHeap, HeapItem{
				seq:         seq,
				streamIndex: i,
				stream:      stream,
			})
		}
	}
	
	return &StreamUnion{
		heap:      sequenceHeap,
		ascending: ascending,
		streams:   criterionStreams,
		lastSeq:   0, // Initialize to 0 for first comparison
	}
}

// Next returns the next sequence from the union of all criteria (OR logic with deduplication)
func (s *StreamUnion) Next() (sequence, bool) {
	for s.heap.Len() > 0 {
		recordHeapOp() // Debug metrics
		item := heap.Pop(s.heap).(HeapItem)
		
		// Deduplication: skip if we've already returned this sequence
		if item.seq != s.lastSeq {
			s.lastSeq = item.seq
			
			// Advance the criterion stream that provided this sequence
			if nextSeq, hasNext := item.stream.Next(); hasNext {
				recordHeapOp() // Debug metrics
				heap.Push(s.heap, HeapItem{
					seq:         nextSeq,
					streamIndex: item.streamIndex,
					stream:      item.stream,
				})
			}
			
			return item.seq, true
		}
		
		// Skip duplicate - advance the stream that provided it
		if nextSeq, hasNext := item.stream.Next(); hasNext {
			recordHeapOp() // Debug metrics
			heap.Push(s.heap, HeapItem{
				seq:         nextSeq,
				streamIndex: item.streamIndex,
				stream:      item.stream,
			})
		}
	}
	
	return 0, false
}

// Close releases all underlying criterion streams
func (s *StreamUnion) Close() error {
	var lastErr error
	for _, stream := range s.streams {
		if err := stream.Close(); err != nil {
			log.Printf("Error closing criterion stream: %v", err)
			lastErr = err
		}
	}
	return lastErr
}

// IsExhausted returns true if all criterion streams are exhausted
func (s *StreamUnion) IsExhausted() bool {
	return s.heap.Len() == 0
}