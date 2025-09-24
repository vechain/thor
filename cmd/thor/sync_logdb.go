// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/pmezard/go-difflib/difflib"
	"gopkg.in/cheggaaa/pb.v1"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func syncLogDB(ctx context.Context, repo *chain.Repository, logDB *logdb.LogDB, verify bool) error {
	startPos, err := seekLogDBSyncPosition(repo, logDB)
	if err != nil {
		return errors.Wrap(err, "seek log db sync position")
	}
	if verify && startPos > 0 {
		if err := verifyLogDB(ctx, startPos-1, repo, logDB); err != nil {
			return errors.Wrap(err, "verify log db")
		}
	}

	best := repo.BestBlockSummary()
	bestNum := best.Header.Number()

	if bestNum == startPos {
		return nil
	}

	if startPos == 0 {
		fmt.Println(">> Rebuilding log db <<", time.Now().String())
		startPos = 1 // block 0 can be skipped
	} else {
		fmt.Println(">> Syncing log db <<")
	}

	pb := pb.New64(int64(bestNum)).
		Set64(int64(startPos - 1)).
		SetMaxWidth(90).
		Start()

	defer func() { pb.NotPrint = true }()

	// Create multiple log writers for parallel processing
	numWorkers := goruntime.NumCPU()
	if numWorkers > 16 {
		numWorkers = 16 // Cap at 8 workers to avoid too many connections
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	fmt.Printf("Using %d workers for parallel log writing\n", numWorkers)

	// Create worker pool for log writing
	type logTask struct {
		block    *block.Block
		receipts tx.Receipts
	}

	logTasks := make(chan logTask, 1000)
	var wg sync.WaitGroup
	var writeErrors []error
	var mu sync.Mutex

	// Start log writing workers with separate connections
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Create separate database connection for this worker
			workerLogDB, err := logdb.New(logDB.Path())
			if err != nil {
				writeErrors = append(writeErrors, fmt.Errorf("worker %d conn error: %v", workerID, err))
			}
			defer workerLogDB.Close()

			w := workerLogDB.NewWriterSyncOff()

			uncommittedCount := 0
			const commitInterval = 100

			for task := range logTasks {
				if err := w.Write(task.block, task.receipts); err != nil {
					mu.Lock()
					writeErrors = append(writeErrors, fmt.Errorf("worker %d error: %v", workerID, err))
					mu.Unlock()
					continue
				}

				uncommittedCount++

				if uncommittedCount >= commitInterval {
					if err := w.Commit(); err != nil {
						mu.Lock()
						writeErrors = append(writeErrors, fmt.Errorf("worker %d commit error: %v", workerID, err))
						mu.Unlock()
					}
					uncommittedCount = 0
				}
			}

			// Final commit
			if uncommittedCount > 0 {
				if err := w.Commit(); err != nil {
					mu.Lock()
					writeErrors = append(writeErrors, fmt.Errorf("worker %d final commit error: %v", workerID, err))
					mu.Unlock()
				}
			}
		}(i)
	}

	var (
		goes    co.Goes
		pumpErr error
		ch      = make(chan *block.Block, 1000)
		cancel  func()
	)

	ctx, cancel = context.WithCancel(ctx)
	defer goes.Wait()
	goes.Go(func() {
		defer close(ch)
		pumpErr = pumpBlockAndReceipts(ctx, repo, best.Header.ID(), startPos, bestNum, ch)
	})

	defer cancel()

	// Process blocks and send to workers
	for b := range ch {
		receipts, err := repo.GetBlockReceipts(b.Header().ID())
		if err != nil {
			close(logTasks)
			wg.Wait()
			return err
		}

		select {
		case logTasks <- logTask{block: b, receipts: receipts}:
			pb.Increment()
		case <-ctx.Done():
			close(logTasks)
			wg.Wait()
			return ctx.Err()
		}
	}

	close(logTasks)
	wg.Wait()

	// Check for write errors
	if len(writeErrors) > 0 {
		return fmt.Errorf("log writing errors: %v", writeErrors)
	}

	if pumpErr != nil {
		return pumpErr
	}

	// Remove the problematic lines - they're not needed for sync
	// if b != nil && stats.processed > 0 {
	//     report(b)
	// }
	pb.Finish()
	return nil
}

func seekLogDBSyncPosition(repo *chain.Repository, logDB *logdb.LogDB) (uint32, error) {
	best := repo.BestBlockSummary().Header
	if best.Number() == 0 {
		return 0, nil
	}

	newestID, err := logDB.NewestBlockID()
	if err != nil {
		return 0, err
	}

	if block.Number(newestID) == 0 {
		return 0, nil
	}

	if newestID == best.ID() {
		return best.Number(), nil
	}

	seekStart := block.Number(newestID)
	if seekStart >= best.Number() {
		seekStart = best.Number() - 1
	}

	header, err := repo.NewChain(best.ID()).GetBlockHeader(seekStart)
	if err != nil {
		return 0, err
	}

	for header.Number() > 0 {
		has, err := logDB.HasBlockID(header.ID())
		if err != nil {
			return 0, err
		}
		if has {
			break
		}

		summary, err := repo.GetBlockSummary(header.ParentID())
		if err != nil {
			return 0, err
		}
		header = summary.Header
	}
	return block.Number(header.ID()) + 1, nil
}

func verifyLogDB(ctx context.Context, endBlockNum uint32, repo *chain.Repository, logDB *logdb.LogDB) error {
	fmt.Println(">> Verifying log db <<")
	pb := pb.New64(int64(endBlockNum)).
		Set64(0).
		SetMaxWidth(90).
		Start()
	defer func() { pb.NotPrint = true }()

	const logStep = uint32(100)

	var (
		best        = repo.BestBlockSummary()
		evLogs      []*logdb.Event
		trLogs      []*logdb.Transfer
		logLimit    = uint32(0)
		splitEvLogs = func(id thor.Bytes32) (logs []*logdb.Event) {
			if len(evLogs) == 0 {
				return
			}
			for i, log := range evLogs {
				if log.BlockID != id {
					if i > 0 {
						logs = evLogs[:i]
						evLogs = evLogs[i:]
					}
					return
				}
			}
			logs = evLogs
			evLogs = nil
			return
		}
		splitTrLogs = func(id thor.Bytes32) (logs []*logdb.Transfer) {
			if len(trLogs) == 0 {
				return
			}
			for i, log := range trLogs {
				if log.BlockID != id {
					if i > 0 {
						logs = trLogs[:i]
						trLogs = trLogs[i:]
					}
					return
				}
			}
			logs = trLogs
			trLogs = nil
			return
		}
	)

	var (
		goes    co.Goes
		pumpErr error
		ch      = make(chan *block.Block, 512)
		cancel  func()
	)

	ctx, cancel = context.WithCancel(ctx)
	defer goes.Wait()
	goes.Go(func() {
		defer close(ch)
		pumpErr = pumpBlockAndReceipts(ctx, repo, best.Header.ID(), 1, endBlockNum, ch)
	})

	defer cancel()

	for b := range ch {
		id := b.Header().ID()
		num := b.Header().Number()
		if num > logLimit {
			var err error
			logLimit += logStep
			evLogs, err = logDB.FilterEvents(context.TODO(), &logdb.EventFilter{
				Range: &logdb.Range{
					From: num,
					To:   logLimit,
				},
			})
			if err != nil {
				return err
			}
			trLogs, err = logDB.FilterTransfers(context.TODO(), &logdb.TransferFilter{
				Range: &logdb.Range{
					From: num,
					To:   logLimit,
				},
			})
			if err != nil {
				return err
			}
		}

		receipts, err := repo.GetBlockReceipts(id)
		if err != nil {
			return err
		}

		if err := verifyLogDBPerBlock(b, receipts, splitEvLogs(id), splitTrLogs(id)); err != nil {
			return err
		}
		pb.Add64(1)

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	pb.Finish()
	return pumpErr
}

func verifyLogDBPerBlock(
	block *block.Block,
	receipts tx.Receipts,
	eventLogs []*logdb.Event,
	transferLogs []*logdb.Transfer,
) error {
	convertTopics := func(topics []thor.Bytes32) (r [5]*thor.Bytes32) {
		for i, t := range topics {
			topic := t
			r[i] = &topic
		}
		return
	}

	n := block.Header().Number()
	id := block.Header().ID()
	ts := block.Header().Timestamp()
	evCount := 0
	trCount := 0

	var expectedEvLogs []*logdb.Event
	var expectedTrLogs []*logdb.Transfer
	txs := block.Transactions()

	evCount = 0
	trCount = 0
	for txIndex, r := range receipts {
		tx := txs[txIndex]
		origin, _ := tx.Origin()

		for clauseIndex, output := range r.Outputs {
			for _, ev := range output.Events {
				var data []byte
				if len(ev.Data) > 0 {
					data = ev.Data
				}
				expectedEvLogs = append(expectedEvLogs, &logdb.Event{
					BlockNumber: n,
					LogIndex:    uint32(evCount),
					BlockID:     id,
					BlockTime:   ts,
					TxID:        tx.ID(),
					TxOrigin:    origin,
					ClauseIndex: uint32(clauseIndex),
					Address:     ev.Address,
					Topics:      convertTopics(ev.Topics),
					Data:        data,
					TxIndex:     uint32(txIndex),
				})
				evCount++
			}
			for _, tr := range output.Transfers {
				expectedTrLogs = append(expectedTrLogs, &logdb.Transfer{
					BlockNumber: n,
					LogIndex:    uint32(trCount),
					BlockID:     id,
					BlockTime:   ts,
					TxID:        tx.ID(),
					TxOrigin:    origin,
					ClauseIndex: uint32(clauseIndex),
					Sender:      tr.Sender,
					Recipient:   tr.Recipient,
					Amount:      tr.Amount,
					TxIndex:     uint32(txIndex),
				})
				trCount++
			}
		}
	}
	if !equalEvents(eventLogs, expectedEvLogs) {
		fmt.Println("\nDiff event logs")
		fmt.Println(jsonDiff(expectedEvLogs, eventLogs))
		return errors.New("incorrect logs")
	}
	if !equalTransfers(transferLogs, expectedTrLogs) {
		fmt.Println("\nDiff transfer logs")
		fmt.Println(jsonDiff(expectedTrLogs, transferLogs))
		return errors.New("incorrect logs")
	}
	return nil
}

// equalEvents performs a statically typed comparison of two Event slices
func equalEvents(a, b []*logdb.Event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalEvent(a[i], b[i]) {
			return false
		}
	}
	return true
}

// equalEvent performs a statically typed comparison of two Event pointers
func equalEvent(a, b *logdb.Event) bool {
	if a == nil || b == nil {
		return a == b
	}

	// fast-fail on all primitive fields
	if a.BlockNumber != b.BlockNumber ||
		a.LogIndex != b.LogIndex ||
		a.BlockTime != b.BlockTime ||
		a.TxIndex != b.TxIndex ||
		a.ClauseIndex != b.ClauseIndex {
		return false
	}

	// compare IDs and addresses via byte-level equality
	if !bytes.Equal(a.BlockID.Bytes(), b.BlockID.Bytes()) ||
		!bytes.Equal(a.TxID.Bytes(), b.TxID.Bytes()) ||
		!bytes.Equal(a.TxOrigin.Bytes(), b.TxOrigin.Bytes()) ||
		!bytes.Equal(a.Address.Bytes(), b.Address.Bytes()) {
		return false
	}

	// topics: nil vs non-nil are unequal, otherwise byte-compare the contents
	if !equalTopics(a.Topics, b.Topics) {
		return false
	}

	// distinguish nil vs empty slice (reflect.DeepEqual would treat them unequal)
	if (a.Data == nil) != (b.Data == nil) {
		return false
	}
	if !bytes.Equal(a.Data, b.Data) {
		return false
	}

	return true
}

// equalTopics compares two [5]*Bytes32 arrays, distinguishing nil pointers
// and falling back to a byte-level compare of the 32-byte contents.
func equalTopics(a, b [5]*thor.Bytes32) bool {
	for i := range a {
		if a[i] == nil || b[i] == nil {
			if a[i] != b[i] {
				return false
			}
			continue
		}
		if !bytes.Equal(a[i].Bytes(), b[i].Bytes()) {
			return false
		}
	}
	return true
}

// equalTransfers performs a statically typed comparison of two Transfer slices
func equalTransfers(a, b []*logdb.Transfer) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalTransfer(a[i], b[i]) {
			return false
		}
	}
	return true
}

// equalTransfer performs a statically typed comparison of two Transfer pointers
// and treats nil Amount as zero for semantic equality.
func equalTransfer(a, b *logdb.Transfer) bool {
	if a == nil || b == nil {
		return a == b
	}

	// fast-fail on primitive fields
	if a.BlockNumber != b.BlockNumber ||
		a.LogIndex != b.LogIndex ||
		a.BlockTime != b.BlockTime ||
		a.TxIndex != b.TxIndex ||
		a.ClauseIndex != b.ClauseIndex {
		return false
	}

	// compare IDs and addresses via byte-level equality
	if !bytes.Equal(a.BlockID.Bytes(), b.BlockID.Bytes()) ||
		!bytes.Equal(a.TxID.Bytes(), b.TxID.Bytes()) ||
		!bytes.Equal(a.TxOrigin.Bytes(), b.TxOrigin.Bytes()) ||
		!bytes.Equal(a.Sender.Bytes(), b.Sender.Bytes()) ||
		!bytes.Equal(a.Recipient.Bytes(), b.Recipient.Bytes()) {
		return false
	}

	// normalize nil<->zero for Amount
	var aAmt, bAmt *big.Int
	if a.Amount == nil || a.Amount.Sign() == 0 {
		aAmt = big.NewInt(0)
	} else {
		aAmt = a.Amount
	}
	if b.Amount == nil || b.Amount.Sign() == 0 {
		bAmt = big.NewInt(0)
	} else {
		bAmt = b.Amount
	}
	if aAmt.Cmp(bAmt) != 0 {
		return false
	}

	return true
}

func jsonDiff(expected, actual any) string {
	e, _ := json.MarshalIndent(expected, "", "  ")
	a, _ := json.MarshalIndent(actual, "", "  ")
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(e)),
		B:        difflib.SplitLines(string(a)),
		FromFile: "Expected",
		FromDate: "",
		ToFile:   "Actual",
		ToDate:   "",
		Context:  1,
	})
	return diff
}

func pumpBlockAndReceipts(ctx context.Context, repo *chain.Repository, headID thor.Bytes32, from, to uint32, ch chan<- *block.Block) error {
	var (
		chain = repo.NewChain(headID)
		buf   []*block.Block
	)
	const bufLen = 256
	for i := from; i <= to; i++ {
		b, err := chain.GetBlock(i)
		if err != nil {
			return err
		}

		buf = append(buf, b)
		if len(buf) >= bufLen {
			select {
			case <-co.Parallel(func(queue chan<- func()) {
				for _, b := range buf {
					queue <- func() {
						b.Header().ID()
					}
					for _, tx := range b.Transactions() {
						queue <- func() {
							tx.ID()
						}
					}
				}
			}):
			case <-ctx.Done():
				return ctx.Err()
			}

			for _, b := range buf {
				select {
				case ch <- b:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			buf = buf[:0]
		}
		// recreate the chain to avoid the internal trie holds too many nodes.
		if n := i - from; n > 0 && n%10000 == 0 {
			chain = repo.NewChain(headID)
		}
	}

	// pump remained blocks
	for _, b := range buf {
		select {
		case ch <- b:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
