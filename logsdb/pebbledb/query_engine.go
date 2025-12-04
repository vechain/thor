// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"context"
	"log"

	"github.com/cockroachdb/pebble"

	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
)

// StreamingQueryEngine handles all query execution with streaming iteration
type StreamingQueryEngine struct {
	db *pebble.DB
	// Reusable buffer for key construction during materialization
	keyBuffer []byte
}

// NewStreamingQueryEngine creates a new query engine
func NewStreamingQueryEngine(db *pebble.DB) *StreamingQueryEngine {
	return &StreamingQueryEngine{
		db:        db,
		keyBuffer: make([]byte, 32), // Pre-allocate buffer for key construction
	}
}

// buildEventPrimaryKey efficiently builds an event primary key using the reusable buffer
func (q *StreamingQueryEngine) buildEventPrimaryKey(seq sequence) []byte {
	q.keyBuffer = q.keyBuffer[:0] // Reset buffer length but keep capacity
	q.keyBuffer = append(q.keyBuffer, eventPrimaryPrefix...)
	q.keyBuffer = append(q.keyBuffer, seq.BigEndianBytes()...)
	return q.keyBuffer
}

// buildTransferPrimaryKey efficiently builds a transfer primary key using the reusable buffer
func (q *StreamingQueryEngine) buildTransferPrimaryKey(seq sequence) []byte {
	q.keyBuffer = q.keyBuffer[:0] // Reset buffer length but keep capacity
	q.keyBuffer = append(q.keyBuffer, transferPrimaryPrefix...)
	q.keyBuffer = append(q.keyBuffer, seq.BigEndianBytes()...)
	return q.keyBuffer
}

// FilterEvents implements event filtering with proper AND/OR semantics
func (q *StreamingQueryEngine) FilterEvents(ctx context.Context, filter *logsdb.EventFilter) (results []*logsdb.Event, err error) {
	// Build criterion-level streams
	criterionStreams, err := q.buildEventCriterionStreams(filter)
	if err != nil {
		return nil, err
	}

	defer func() {
		// Guaranteed cleanup regardless of success/failure
		q.closeEventStreams(criterionStreams)

		// Handle context cancellation cleanup
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()

	// Execute streaming query (always in ASC order internally)
	sequences, err := q.executeEventStreaming(ctx, criterionStreams, filter.Options, filter.Order)
	if err != nil {
		return nil, err
	}

	// For DESC queries: reverse collected sequences, then apply offset/limit
	if filter.Order == logsdb.DESC {
		sequences = q.reverseAndApplyLimits(sequences, filter.Options.Offset, filter.Options.Limit)
	}

	// Materialization step
	return q.materializeEvents(sequences)
}

// FilterTransfers implements transfer filtering with proper AND/OR semantics
func (q *StreamingQueryEngine) FilterTransfers(ctx context.Context, filter *logsdb.TransferFilter) (results []*logsdb.Transfer, err error) {
	criterionStreams, err := q.buildTransferCriterionStreams(filter)
	if err != nil {
		return nil, err
	}

	defer func() {
		q.closeTransferStreams(criterionStreams)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()

	sequences, err := q.executeTransferStreaming(ctx, criterionStreams, filter.Options, filter.Order)
	if err != nil {
		return nil, err
	}

	if filter.Order == logsdb.DESC {
		sequences = q.reverseAndApplyLimits(sequences, filter.Options.Offset, filter.Options.Limit)
	}

	return q.materializeTransfers(sequences)
}

// buildEventCriterionStreams creates criterion-level intersectors for events
func (q *StreamingQueryEngine) buildEventCriterionStreams(filter *logsdb.EventFilter) ([]*StreamIntersector, error) {
	// Handle empty CriteriaSet with synthetic range-only criterion
	criteria := filter.CriteriaSet
	if len(criteria) == 0 {
		criteria = []*logsdb.EventCriteria{{}} // synthetic range-only criterion
	}

	minSeq, maxSeq := sequenceRangeFromRange(filter.Range)
	var criterionStreams []*StreamIntersector

	// Fast path: single-criterion queries
	if len(criteria) == 1 {
		criterion := criteria[0]
		if isSingleAddressOnly(criterion) {
			// Single address-only fast path
			iter := q.createAddressRangeIterator(*criterion.Address, minSeq, maxSeq, true)
			intersector := NewStreamIntersector([]*StreamIterator{iter}, true)
			return []*StreamIntersector{intersector}, nil
		}
	}

	for _, criterion := range criteria {
		var iterators []*StreamIterator

		// Address iterator
		if criterion.Address != nil {
			iter := q.createAddressIterator(*criterion.Address, minSeq, maxSeq, true) // Always ASC
			iterators = append(iterators, iter)
		}

		// Topic iterators
		for i, topic := range criterion.Topics {
			if topic != nil {
				iter := q.createTopicIterator(i, *topic, minSeq, maxSeq, true) // Always ASC
				iterators = append(iterators, iter)
			}
		}

		// If no address/topics (range-only criterion), use dense sequence index if available
		if len(iterators) == 0 {
			if q.hasSequenceIndexes() {
				iter := q.createEventSequenceIterator(minSeq, maxSeq, true) // Always ASC
				iterators = append(iterators, iter)
			} else {
				// Fallback to existing primary range iterator for backward compatibility
				iter := q.createPrimaryRangeIteratorEvents(minSeq, maxSeq, true) // Always ASC
				iterators = append(iterators, iter)
			}
		}

		criterionStream := NewStreamIntersector(iterators, true) // Always ASC
		criterionStreams = append(criterionStreams, criterionStream)
	}

	return criterionStreams, nil
}

// buildTransferCriterionStreams creates criterion-level intersectors for transfers
func (q *StreamingQueryEngine) buildTransferCriterionStreams(filter *logsdb.TransferFilter) ([]*StreamIntersector, error) {
	// Handle empty CriteriaSet with synthetic range-only criterion
	criteria := filter.CriteriaSet
	if len(criteria) == 0 {
		criteria = []*logsdb.TransferCriteria{{}} // synthetic range-only criterion
	}

	minSeq, maxSeq := sequenceRangeFromRange(filter.Range)
	var criterionStreams []*StreamIntersector

	// Fast path: single-criterion queries
	if len(criteria) == 1 {
		criterion := criteria[0]
		if isSingleSenderOnly(criterion) {
			iter := q.createSenderRangeIterator(*criterion.Sender, minSeq, maxSeq, true)
			intersector := NewStreamIntersector([]*StreamIterator{iter}, true)
			return []*StreamIntersector{intersector}, nil
		}
		if isSingleRecipientOnly(criterion) {
			iter := q.createRecipientRangeIterator(*criterion.Recipient, minSeq, maxSeq, true)
			intersector := NewStreamIntersector([]*StreamIterator{iter}, true)
			return []*StreamIntersector{intersector}, nil
		}
		if isSingleTxOriginOnly(criterion) {
			iter := q.createTxOriginRangeIterator(*criterion.TxOrigin, minSeq, maxSeq, true)
			intersector := NewStreamIntersector([]*StreamIterator{iter}, true)
			return []*StreamIntersector{intersector}, nil
		}
	}

	for _, criterion := range criteria {
		var iterators []*StreamIterator

		if criterion.Sender != nil {
			iter := q.createTransferSenderIterator(*criterion.Sender, minSeq, maxSeq, true)
			iterators = append(iterators, iter)
		}

		if criterion.Recipient != nil {
			iter := q.createTransferRecipientIterator(*criterion.Recipient, minSeq, maxSeq, true)
			iterators = append(iterators, iter)
		}

		if criterion.TxOrigin != nil {
			iter := q.createTransferTxOriginIterator(*criterion.TxOrigin, minSeq, maxSeq, true)
			iterators = append(iterators, iter)
		}

		if len(iterators) == 0 {
			if q.hasSequenceIndexes() {
				iter := q.createTransferSequenceIterator(minSeq, maxSeq, true)
				iterators = append(iterators, iter)
			} else {
				// Fallback to existing primary range iterator for backward compatibility
				iter := q.createPrimaryRangeIteratorTransfers(minSeq, maxSeq, true)
				iterators = append(iterators, iter)
			}
		}

		criterionStream := NewStreamIntersector(iterators, true)
		criterionStreams = append(criterionStreams, criterionStream)
	}

	return criterionStreams, nil
}

// executeEventStreaming executes the streaming query for events with corrected signature
func (q *StreamingQueryEngine) executeEventStreaming(
	ctx context.Context,
	intersectors []*StreamIntersector,
	options *logsdb.Options,
	order logsdb.Order,
) ([]sequence, error) {
	var sequences []sequence

	if len(intersectors) == 1 {
		// Single intersector fast path - no union overhead
		intersector := intersectors[0]

		if order == logsdb.ASC {
			// Skip offset items
			for i := 0; i < int(options.Offset); i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				if _, ok := intersector.Next(); !ok {
					break
				}
			}

			// Collect limit items
			for i := 0; i < int(options.Limit); i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				seq, ok := intersector.Next()
				if !ok {
					break
				}
				sequences = append(sequences, seq)
			}
		} else { // DESC
			// Collect offset+limit in ASC order, FilterEvents will handle reversal
			maxItems := int(options.Offset + options.Limit)
			for range maxItems {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				seq, ok := intersector.Next()
				if !ok {
					break
				}
				sequences = append(sequences, seq)
			}
		}
	} else {
		// Multi-intersector union logic
		union := NewStreamUnion(intersectors, true) // Always ASC internally
		defer union.Close()

		if order == logsdb.ASC {
			// Skip offset items
			for i := 0; i < int(options.Offset); i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				if _, ok := union.Next(); !ok {
					break
				}
			}

			// Collect limit items
			for i := 0; i < int(options.Limit); i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				seq, ok := union.Next()
				if !ok {
					break
				}
				sequences = append(sequences, seq)
			}
		} else { // DESC
			// Collect offset+limit in ASC order, FilterEvents will handle reversal
			maxItems := int(options.Offset + options.Limit)
			for range maxItems {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				seq, ok := union.Next()
				if !ok {
					break
				}
				sequences = append(sequences, seq)
			}
		}
	}

	return sequences, nil // Always returns ASC order
}

// executeTransferStreaming executes the streaming query for transfers with corrected signature
func (q *StreamingQueryEngine) executeTransferStreaming(
	ctx context.Context,
	intersectors []*StreamIntersector,
	options *logsdb.Options,
	order logsdb.Order,
) ([]sequence, error) {
	var sequences []sequence

	if len(intersectors) == 1 {
		// Single intersector fast path - no union overhead
		intersector := intersectors[0]

		if order == logsdb.ASC {
			// Skip offset items
			for i := 0; i < int(options.Offset); i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				if _, ok := intersector.Next(); !ok {
					break
				}
			}

			// Collect limit items
			for i := 0; i < int(options.Limit); i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				seq, ok := intersector.Next()
				if !ok {
					break
				}
				sequences = append(sequences, seq)
			}
		} else { // DESC
			// Collect offset+limit in ASC order, FilterTransfers will handle reversal
			maxItems := int(options.Offset + options.Limit)
			for range maxItems {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				seq, ok := intersector.Next()
				if !ok {
					break
				}
				sequences = append(sequences, seq)
			}
		}
	} else {
		// Multi-intersector union logic
		union := NewStreamUnion(intersectors, true) // Always ASC internally
		defer union.Close()

		if order == logsdb.ASC {
			// Skip offset items
			for i := 0; i < int(options.Offset); i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				if _, ok := union.Next(); !ok {
					break
				}
			}

			// Collect limit items
			for i := 0; i < int(options.Limit); i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				seq, ok := union.Next()
				if !ok {
					break
				}
				sequences = append(sequences, seq)
			}
		} else { // DESC
			// Collect offset+limit in ASC order, FilterTransfers will handle reversal
			maxItems := int(options.Offset + options.Limit)
			for range maxItems {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				seq, ok := union.Next()
				if !ok {
					break
				}
				sequences = append(sequences, seq)
			}
		}
	}

	return sequences, nil // Always returns ASC order
}

// reverseAndApplyLimits reverses sequences for DESC order and applies offset/limit
func (q *StreamingQueryEngine) reverseAndApplyLimits(sequences []sequence, offset, limit uint64) []sequence {
	// Reverse slice
	for i, j := 0, len(sequences)-1; i < j; i, j = i+1, j-1 {
		sequences[i], sequences[j] = sequences[j], sequences[i]
	}

	// Apply offset and limit
	start := int(offset)
	if start >= len(sequences) {
		return []sequence{}
	}

	end := min(start+int(limit), len(sequences))

	return sequences[start:end]
}

// sequenceRangeFromRange calculates sequence range from block range - unified helper
func sequenceRangeFromRange(r *logsdb.Range) (sequence, sequence) {
	if r == nil {
		return 0, MaxSequenceValue
	}
	minSeq, _ := newSequence(r.From, 0, 0)
	maxSeq, _ := newSequence(r.To, txIndexMask, logIndexMask)
	return minSeq, maxSeq
}

// Iterator creation methods

// performIteratorSeek performs initial seek based on direction
func performIteratorSeek(iter *pebble.Iterator, lowerBound, upperBound []byte, ascending bool) {
	if ascending {
		recordSeekGE() // Existing debug metrics
		iter.SeekGE(lowerBound)
	} else {
		recordSeekLT() // Existing debug metrics
		iter.SeekLT(upperBound)
	}
}

func (q *StreamingQueryEngine) createAddressIterator(addr thor.Address, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	lowerBound := eventAddressKey(addr, minSeq)
	upperBound := eventAddressKey(addr, maxSeq.Next())

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating address iterator: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

func (q *StreamingQueryEngine) createTopicIterator(topicIndex int, topic thor.Bytes32, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	lowerBound := eventTopicKey(topicIndex, topic, minSeq)
	upperBound := eventTopicKey(topicIndex, topic, maxSeq.Next())

	if lowerBound == nil || upperBound == nil {
		return NewExhaustedStreamIterator()
	}

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating topic iterator: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

func (q *StreamingQueryEngine) createTransferSenderIterator(addr thor.Address, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	lowerBound := transferSenderKey(addr, minSeq)
	upperBound := transferSenderKey(addr, maxSeq.Next())

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating transfer sender iterator: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

func (q *StreamingQueryEngine) createTransferRecipientIterator(addr thor.Address, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	lowerBound := transferRecipientKey(addr, minSeq)
	upperBound := transferRecipientKey(addr, maxSeq.Next())

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating transfer recipient iterator: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

func (q *StreamingQueryEngine) createTransferTxOriginIterator(addr thor.Address, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	lowerBound := transferTxOriginKey(addr, minSeq)
	upperBound := transferTxOriginKey(addr, maxSeq.Next())

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating transfer tx origin iterator: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

func (q *StreamingQueryEngine) createPrimaryRangeIteratorEvents(minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	// Primary key layout: E/<seq>
	// Index key layout: EA/<address>/<seq>, ET0/<topic>/<seq>, etc.
	// We need to ensure we only read primary keys (E/*) and not index keys (EA/*, ET0/*, etc.)
	lowerBound := eventPrimaryKey(minSeq)

	// For event primary range, use "F" as upper bound to stop before any transfer keys (T*)
	// Since E=0x45 and F=0x46, this will include all E+sequence keys but exclude T+sequence keys
	// This is more efficient than scanning the entire database and filtering manually
	upperBound := []byte("F")

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating primary range iterator for events: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewEventPrimaryStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

func (q *StreamingQueryEngine) createPrimaryRangeIteratorTransfers(minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	// Primary key layout: T/<seq>
	// Index key layout: TO/<txOrigin>/<seq>, TR/<recipient>/<seq>, TS/<sender>/<seq>
	// We need to ensure we only read primary keys (T/*) and not index keys (TO/*, TR/*, TS/*)
	lowerBound := transferPrimaryKey(minSeq)

	// For transfer primary range, use "U" as upper bound to stop after all T+sequence keys
	// Since T=0x54 and U=0x55, this will include all T+sequence keys
	// This is more efficient than scanning the entire database and filtering manually
	upperBound := []byte("U")

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating primary range iterator for transfers: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewTransferPrimaryStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

// Fast path range iterator constructors

func (q *StreamingQueryEngine) createAddressRangeIterator(addr thor.Address, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	// Use same implementation as createAddressIterator
	return q.createAddressIterator(addr, minSeq, maxSeq, ascending)
}

func (q *StreamingQueryEngine) createSenderRangeIterator(sender thor.Address, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	return q.createTransferSenderIterator(sender, minSeq, maxSeq, ascending)
}

func (q *StreamingQueryEngine) createRecipientRangeIterator(recipient thor.Address, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	return q.createTransferRecipientIterator(recipient, minSeq, maxSeq, ascending)
}

func (q *StreamingQueryEngine) createTxOriginRangeIterator(txOrigin thor.Address, minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	return q.createTransferTxOriginIterator(txOrigin, minSeq, maxSeq, ascending)
}

// Cleanup methods

func (q *StreamingQueryEngine) closeEventStreams(streams []*StreamIntersector) {
	for _, stream := range streams {
		if err := stream.Close(); err != nil {
			log.Printf("Error closing event stream: %v", err)
		}
	}
}

func (q *StreamingQueryEngine) closeTransferStreams(streams []*StreamIntersector) {
	for _, stream := range streams {
		if err := stream.Close(); err != nil {
			log.Printf("Error closing transfer stream: %v", err)
		}
	}
}

// Dense sequence index iterator creators

// createEventSequenceIterator creates an iterator for ES/ dense sequence index
func (q *StreamingQueryEngine) createEventSequenceIterator(minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	lowerBound := eventSequenceKey(minSeq)

	var upperBound []byte
	if maxSeq == MaxSequenceValue {
		// Use prefix upper bound instead of trying to increment max value
		upperBound = []byte("ET") // Next prefix after "ES"
	} else {
		upperBound = eventSequenceKey(maxSeq.Next())
	}

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating event sequence iterator: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

// createTransferSequenceIterator creates an iterator for TSX/ dense sequence index
func (q *StreamingQueryEngine) createTransferSequenceIterator(minSeq, maxSeq sequence, ascending bool) *StreamIterator {
	lowerBound := transferSequenceKey(minSeq)

	var upperBound []byte
	if maxSeq == MaxSequenceValue {
		// Use prefix upper bound instead of trying to increment max value
		upperBound = []byte("U") // Next prefix after "TSX"
	} else {
		upperBound = transferSequenceKey(maxSeq.Next())
	}

	opts := &pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		log.Printf("Error creating transfer sequence iterator: %v", err)
		return NewExhaustedStreamIterator()
	}

	performIteratorSeek(iter, lowerBound, upperBound, ascending)

	return NewStreamIterator(iter, minSeq, maxSeq, ascending, lowerBound, upperBound)
}

// hasSequenceIndexes performs a minimal check for ES/ prefix existence
func (q *StreamingQueryEngine) hasSequenceIndexes() bool {
	opts := &pebble.IterOptions{
		LowerBound: []byte("ES"),
		UpperBound: []byte("ET0"), // Corrected upper bound - stops before ET0/ topic indexes
	}

	iter, err := q.db.NewIter(opts)
	if err != nil {
		return false
	}
	defer iter.Close()

	iter.First()
	return iter.Valid()
}

// Helper functions for fast path detection

func isSingleAddressOnly(criterion *logsdb.EventCriteria) bool {
	if criterion.Address == nil {
		return false
	}
	// All topics must be nil
	for _, topic := range criterion.Topics {
		if topic != nil {
			return false
		}
	}
	return true
}

func isSingleSenderOnly(criterion *logsdb.TransferCriteria) bool {
	return criterion.Sender != nil && criterion.Recipient == nil && criterion.TxOrigin == nil
}

func isSingleRecipientOnly(criterion *logsdb.TransferCriteria) bool {
	return criterion.Recipient != nil && criterion.Sender == nil && criterion.TxOrigin == nil
}

func isSingleTxOriginOnly(criterion *logsdb.TransferCriteria) bool {
	return criterion.TxOrigin != nil && criterion.Sender == nil && criterion.Recipient == nil
}
