// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package sqlitedb

import (
	"context"
	"database/sql"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Writer is the transactional log writer for SQLite.
type Writer struct {
	conn      *sql.Conn
	stmtCache *stmtCache

	tx               *sql.Tx
	uncommittedCount int
}

// Truncate truncates the database by deleting logs after blockNum (included).
func (w *Writer) Truncate(blockNum uint32) error {
	seq, err := newSequence(blockNum, 0, 0)
	if err != nil {
		return err
	}

	if err := w.exec("DELETE FROM event WHERE seq >= ?", seq); err != nil {
		return err
	}
	if err := w.exec("DELETE FROM transfer WHERE seq >= ?", seq); err != nil {
		return err
	}
	return nil
}

// Write writes all logs of the given block.
func (w *Writer) Write(b *block.Block, receipts tx.Receipts) error {
	var (
		blockID        = b.Header().ID()
		blockNum       = b.Header().Number()
		blockTimestamp = b.Header().Timestamp()
		txs            = b.Transactions()
		isReceiptEmpty = func(r *tx.Receipt) bool {
			for _, o := range r.Outputs {
				if len(o.Events) > 0 || len(o.Transfers) > 0 {
					return false
				}
			}
			return true
		}
		blockIDInserted bool
	)

	eventCount, transferCount := uint32(0), uint32(0)
	for i, r := range receipts {
		if isReceiptEmpty(r) {
			continue
		}

		if !blockIDInserted {
			// block id is not yet inserted
			if err := w.exec(
				"INSERT OR IGNORE INTO ref(data) VALUES(?)",
				blockID[:]); err != nil {
				return err
			}
			blockIDInserted = true
		}

		var (
			txID     thor.Bytes32
			txOrigin thor.Address
		)
		if i < len(txs) { // block 0 has no tx, but has receipts
			tx := txs[i]
			txID = tx.ID()
			txOrigin, _ = tx.Origin()
		}

		txIndex := i
		if err := w.exec(
			"INSERT OR IGNORE INTO ref(data) VALUES(?),(?)",
			txID[:], txOrigin[:]); err != nil {
			return err
		}

		for clauseIndex, output := range r.Outputs {
			for _, ev := range output.Events {
				if err := w.exec(
					"INSERT OR IGNORE INTO ref (data) VALUES(?),(?),(?),(?),(?),(?)",
					ev.Address[:],
					topicValue(ev.Topics, 0),
					topicValue(ev.Topics, 1),
					topicValue(ev.Topics, 2),
					topicValue(ev.Topics, 3),
					topicValue(ev.Topics, 4),
				); err != nil {
					return err
				}

				const query = "INSERT OR IGNORE INTO event(seq, blockTime, clauseIndex, data, blockID, txID, txOrigin, address, topic0, topic1, topic2, topic3, topic4) " +
					"VALUES(?,?,?,?," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + ")"

				var eventData []byte
				if len(ev.Data) > 0 {
					eventData = ev.Data
				}

				seq, err := newSequence(blockNum, uint32(txIndex), eventCount)
				if err != nil {
					return err
				}

				if err := w.exec(
					query,
					seq,
					blockTimestamp,
					clauseIndex,
					eventData,
					blockID[:],
					txID[:],
					txOrigin[:],
					ev.Address[:],
					topicValue(ev.Topics, 0),
					topicValue(ev.Topics, 1),
					topicValue(ev.Topics, 2),
					topicValue(ev.Topics, 3),
					topicValue(ev.Topics, 4)); err != nil {
					return err
				}
				eventCount++
			}

			for _, tr := range output.Transfers {
				if err := w.exec(
					"INSERT OR IGNORE INTO ref (data) VALUES(?),(?)",
					tr.Sender[:],
					tr.Recipient[:]); err != nil {
					return err
				}
				const query = "INSERT OR IGNORE INTO transfer(seq, blockTime, clauseIndex, amount, blockID, txID, txOrigin, sender, recipient) " +
					"VALUES(?,?,?,?," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + "," +
					refIDQuery + ")"

				seq, err := newSequence(blockNum, uint32(txIndex), transferCount)
				if err != nil {
					return err
				}

				if err := w.exec(
					query,
					seq,
					blockTimestamp,
					clauseIndex,
					tr.Amount.Bytes(),
					blockID[:],
					txID[:],
					txOrigin[:],
					tr.Sender[:],
					tr.Recipient[:]); err != nil {
					return err
				}
				transferCount++
			}
		}
	}
	return nil
}

// Commit commits accumulated logs.
func (w *Writer) Commit() (err error) {
	if w.tx == nil {
		return nil
	}

	defer func() {
		if err == nil {
			w.tx = nil
			w.uncommittedCount = 0
		}
	}()
	return w.tx.Commit()
}

// Rollback rollback all uncommitted logs.
func (w *Writer) Rollback() (err error) {
	if w.tx == nil {
		return nil
	}
	defer func() {
		if err == nil {
			w.tx = nil
			w.uncommittedCount = 0
		}
	}()
	return w.tx.Rollback()
}

// UncommittedCount returns the count of uncommitted logs.
func (w *Writer) UncommittedCount() int {
	return w.uncommittedCount
}

func (w *Writer) exec(query string, args ...any) (err error) {
	if w.tx == nil {
		if w.tx, err = w.conn.BeginTx(context.Background(), nil); err != nil {
			return
		}
	}
	if _, err = w.tx.Stmt(w.stmtCache.MustPrepare(query)).Exec(args...); err != nil {
		return
	}
	w.uncommittedCount++
	return nil
}
