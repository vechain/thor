// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pebbledb

import (
	"github.com/cockroachdb/pebble"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var _ logsdb.Writer = (*PebbleV3Writer)(nil)

// PebbleV3Writer implements logsdb.Writer interface for PebbleDB v3
type PebbleV3Writer struct {
	db        *pebble.DB
	batch     *pebble.Batch
	batchSize int
}

// NewPebbleV3Writer creates a new writer instance
func NewPebbleV3Writer(db *pebble.DB) *PebbleV3Writer {
	return &PebbleV3Writer{
		db:    db,
		batch: db.NewBatch(),
	}
}

// Write writes all logs of the given block
func (w *PebbleV3Writer) Write(b *block.Block, receipts tx.Receipts) error {
	blockNum := b.Header().Number()
	blockID := b.Header().ID()
	blockTime := b.Header().Timestamp()

	eventIdx := uint32(0)
	transferIdx := uint32(0)

	for txIdx, receipt := range receipts {
		if w.isReceiptEmpty(receipt) {
			continue
		}

		txID, txOrigin := w.extractTxInfo(b, txIdx)

		for clauseIdx, output := range receipt.Outputs {
			// Process events
			for _, event := range output.Events {
				seq, err := newSequence(blockNum, uint32(txIdx), eventIdx)
				if err != nil {
					return err
				}

				if err := w.writeEvent(seq, blockID, blockNum, blockTime, txID, uint32(txIdx), txOrigin, uint32(clauseIdx), eventIdx, event); err != nil {
					return err
				}

				eventIdx++
			}

			// Process transfers
			for _, transfer := range output.Transfers {
				seq, err := newSequence(blockNum, uint32(txIdx), transferIdx)
				if err != nil {
					return err
				}

				if err := w.writeTransfer(seq, blockID, blockNum, blockTime, txID, uint32(txIdx), txOrigin, uint32(clauseIdx), transferIdx, transfer); err != nil {
					return err
				}

				transferIdx++
			}
		}
	}

	return nil
}

// writeEvent writes event and all its indexes
func (w *PebbleV3Writer) writeEvent(seq sequence, blockID thor.Bytes32, blockNum uint32, blockTime uint64,
	txID thor.Bytes32, txIdx uint32, txOrigin thor.Address, clauseIdx uint32, logIdx uint32, event *tx.Event) error {

	// Create event record
	eventRecord := &EventRecord{
		BlockID:     blockID,
		BlockNumber: blockNum,
		BlockTime:   blockTime,
		TxID:        txID,
		TxIndex:     txIdx,
		TxOrigin:    txOrigin,
		ClauseIndex: clauseIdx,
		LogIndex:    logIdx,
		Address:     event.Address,
		Topics:      event.Topics,
		Data:        event.Data,
	}

	// Primary storage: E/<seq>
	primaryData, err := eventRecord.RLPEncode()
	if err != nil {
		return err
	}
	w.batch.Set(eventPrimaryKey(seq), primaryData, nil)
	w.batchSize++

	// Address index: EA/<address>/<seq>
	w.batch.Set(eventAddressKey(event.Address, seq), nil, nil)
	w.batchSize++

	// Topic indexes: ET0-ET4/<topic>/<seq> (only for non-empty topics)
	for i, topic := range event.Topics {
		if i >= 5 {
			break // Only index first 5 topics
		}
		// Check for non-zero topic (empty thor.Bytes32 is all zeros)
		if !isZeroBytes32(topic) {
			w.batch.Set(eventTopicKey(i, topic, seq), nil, nil)
			w.batchSize++
		}
	}

	return nil
}

// writeTransfer writes transfer and all its indexes
func (w *PebbleV3Writer) writeTransfer(seq sequence, blockID thor.Bytes32, blockNum uint32, blockTime uint64,
	txID thor.Bytes32, txIdx uint32, txOrigin thor.Address, clauseIdx uint32, logIdx uint32, transfer *tx.Transfer) error {

	// Create transfer record
	transferRecord := &TransferRecord{
		BlockID:     blockID,
		BlockNumber: blockNum,
		BlockTime:   blockTime,
		TxID:        txID,
		TxIndex:     txIdx,
		TxOrigin:    txOrigin,
		ClauseIndex: clauseIdx,
		LogIndex:    logIdx,
		Sender:      transfer.Sender,
		Recipient:   transfer.Recipient,
		Amount:      transfer.Amount,
	}

	// Primary storage: T/<seq>
	primaryData, err := transferRecord.RLPEncode()
	if err != nil {
		return err
	}
	w.batch.Set(transferPrimaryKey(seq), primaryData, nil)
	w.batchSize++

	// Transfer indexes
	w.batch.Set(transferSenderKey(transfer.Sender, seq), nil, nil)
	w.batchSize++

	w.batch.Set(transferRecipientKey(transfer.Recipient, seq), nil, nil)
	w.batchSize++

	w.batch.Set(transferTxOriginKey(txOrigin, seq), nil, nil)
	w.batchSize++

	return nil
}

// Commit commits accumulated logs with sync
func (w *PebbleV3Writer) Commit() error {
	return w.commitWithSync(pebble.Sync)
}

// CommitNoSync commits accumulated logs without sync for bulk loading performance
func (w *PebbleV3Writer) CommitNoSync() error {
	return w.commitWithSync(pebble.NoSync)
}

// commitWithSync commits with the specified write options
func (w *PebbleV3Writer) commitWithSync(opts *pebble.WriteOptions) error {
	if w.batch.Count() == 0 {
		return nil
	}

	err := w.db.Apply(w.batch, opts)
	if err == nil {
		w.batch.Reset()
		w.batchSize = 0
	}
	return err
}

// Rollback rollbacks all uncommitted logs
func (w *PebbleV3Writer) Rollback() error {
	w.batch.Reset()
	w.batchSize = 0
	return nil
}

// UncommittedCount returns the count of uncommitted operations
func (w *PebbleV3Writer) UncommittedCount() int {
	return w.batchSize
}

// Truncate truncates the database by deleting logs after blockNum (included)
// Uses Option A (prefix scan) for indexes as specified in the plan
func (w *PebbleV3Writer) Truncate(blockNum uint32) error {
	minSeq, err := newSequence(blockNum, 0, 0)
	if err != nil {
		return err
	}

	// Create precise bounds for range deletion

	// Delete primary data using range delete
	// IMPORTANT: Use precise bounds that only cover primary keys, not indexes
	eventStartKey := eventPrimaryKey(minSeq)

	// For end bound, use "EA" which is the first possible index prefix after "E"
	// This ensures we stop right before any index keys
	eventEndKey := []byte("EA")

	transferStartKey := transferPrimaryKey(minSeq)
	// Similarly for transfers, use "TO" which is the first transfer index prefix
	transferEndKey := []byte("TO")

	// Use range delete only for primary keys - precise bounds avoid deleting indexes
	w.batch.DeleteRange(eventStartKey, eventEndKey, nil)
	w.batch.DeleteRange(transferStartKey, transferEndKey, nil)

	// Delete indexes using chunked prefix scan (Option A)
	indexPrefixes := []string{
		eventAddrPrefix, eventTopic0Prefix, eventTopic1Prefix, eventTopic2Prefix, eventTopic3Prefix, eventTopic4Prefix,
		transferSenderPrefix, transferRecipientPrefix, transferTxOriginPrefix,
	}

	for _, prefix := range indexPrefixes {
		if err := w.truncateIndexPrefixChunked(prefix, minSeq); err != nil {
			return err
		}
	}

	return nil
}

const truncateChunkSize = 10000

// truncateIndexPrefixChunked deletes index entries in chunks to avoid huge batches
func (w *PebbleV3Writer) truncateIndexPrefixChunked(prefix string, minSeq sequence) error {
	opts := &pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: []byte(prefix + "\xff"),
	}

	iter, err := w.db.NewIter(opts)
	if err != nil {
		return err
	}
	defer iter.Close()

	deleteCount := 0
	preserveCount := 0
	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		seq := sequenceFromKey(key)
		if seq >= minSeq {
			w.batch.Delete(key, nil)
			deleteCount++

			// Commit periodically to avoid huge batches on mainnet data
			if deleteCount%truncateChunkSize == 0 {
				if err := w.Commit(); err != nil {
					return err
				}
			}
		} else {
			preserveCount++
		}
	}

	return iter.Error()
}

// Helper methods

func (w *PebbleV3Writer) isReceiptEmpty(r *tx.Receipt) bool {
	for _, o := range r.Outputs {
		if len(o.Events) > 0 || len(o.Transfers) > 0 {
			return false
		}
	}
	return true
}

func (w *PebbleV3Writer) extractTxInfo(b *block.Block, txIdx int) (thor.Bytes32, thor.Address) {
	txs := b.Transactions()
	if txIdx < len(txs) {
		tx := txs[txIdx]
		txID := tx.ID()
		txOrigin, _ := tx.Origin()
		return txID, txOrigin
	}
	// Block 0 has no transactions but has receipts
	return thor.Bytes32{}, thor.Address{}
}

// WriteMigrationEvents writes events while preserving their original metadata
func (w *PebbleV3Writer) WriteMigrationEvents(events []*logsdb.Event) error {
	for _, event := range events {
		// Create sequence from original event metadata
		seq, err := newSequence(event.BlockNumber, event.TxIndex, event.LogIndex)
		if err != nil {
			return err
		}

		// Create event record preserving all original metadata
		eventRecord := NewEventRecord(event)

		// Primary storage
		primaryData, err := eventRecord.RLPEncode()
		if err != nil {
			return err
		}
		w.batch.Set(eventPrimaryKey(seq), primaryData, nil)
		w.batchSize++

		// Address index
		w.batch.Set(eventAddressKey(event.Address, seq), nil, nil)
		w.batchSize++

		// Topic indexes (only for non-empty topics)
		for i, topic := range event.Topics {
			if i >= 5 {
				break
			}
			if topic != nil && !isZeroBytes32(*topic) {
				w.batch.Set(eventTopicKey(i, *topic, seq), nil, nil)
				w.batchSize++
			}
		}
	}
	return nil
}

// WriteMigrationTransfers writes transfers while preserving their original metadata
func (w *PebbleV3Writer) WriteMigrationTransfers(transfers []*logsdb.Transfer) error {
	for _, transfer := range transfers {
		// Create sequence from original transfer metadata
		seq, err := newSequence(transfer.BlockNumber, transfer.TxIndex, transfer.LogIndex)
		if err != nil {
			return err
		}

		// Create transfer record preserving all original metadata
		transferRecord := NewTransferRecord(transfer)

		// Primary storage
		primaryData, err := transferRecord.RLPEncode()
		if err != nil {
			return err
		}
		w.batch.Set(transferPrimaryKey(seq), primaryData, nil)
		w.batchSize++

		// Transfer indexes
		w.batch.Set(transferSenderKey(transfer.Sender, seq), nil, nil)
		w.batchSize++

		w.batch.Set(transferRecipientKey(transfer.Recipient, seq), nil, nil)
		w.batchSize++

		w.batch.Set(transferTxOriginKey(transfer.TxOrigin, seq), nil, nil)
		w.batchSize++
	}
	return nil
}

// isZeroBytes32 checks if a thor.Bytes32 is all zeros
func isZeroBytes32(b thor.Bytes32) bool {
	var zero thor.Bytes32
	return b == zero
}
