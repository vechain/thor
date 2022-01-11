// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"gopkg.in/cheggaaa/pb.v1"
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
		fmt.Println(">> Rebuilding log db <<")
		startPos = 1 // block 0 can be skipped
	} else {
		fmt.Println(">> Syncing log db <<")
	}

	pb := pb.New64(int64(bestNum)).
		Set64(int64(startPos - 1)).
		SetMaxWidth(90).
		Start()

	defer func() { pb.NotPrint = true }()

	w := logDB.NewWriterSyncOff()

	if err := w.Truncate(startPos); err != nil {
		return err
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

	for b := range ch {
		receipts, err := repo.GetBlockReceipts(b.Header().ID())
		if err != nil {
			return err
		}
		if err := w.Write(b, receipts); err != nil {
			return err
		}
		if w.UncommittedCount() > 2048 {
			if err := w.Commit(); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			if err := w.Commit(); err != nil {
				return err
			}
			return ctx.Err()
		default:
		}
		pb.Add64(1)
	}
	if err := w.Commit(); err != nil {
		return err
	}
	pb.Finish()
	return pumpErr
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
	transferLogs []*logdb.Transfer) error {

	convertTopics := func(topics []thor.Bytes32) (r [5]*thor.Bytes32) {
		for i, t := range topics {
			t := t
			r[i] = &t
		}
		return
	}

	n := block.Header().Number()
	id := block.Header().ID()
	ts := block.Header().Timestamp()

	var expectedEvLogs []*logdb.Event
	var expectedTrLogs []*logdb.Transfer
	txs := block.Transactions()
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
					Index:       uint32(len(expectedEvLogs)),
					BlockID:     id,
					BlockTime:   ts,
					TxID:        tx.ID(),
					TxOrigin:    origin,
					ClauseIndex: uint32(clauseIndex),
					Address:     ev.Address,
					Topics:      convertTopics(ev.Topics),
					Data:        data,
				})
			}
			for _, tr := range output.Transfers {
				expectedTrLogs = append(expectedTrLogs, &logdb.Transfer{
					BlockNumber: n,
					Index:       uint32(len(expectedTrLogs)),
					BlockID:     id,
					BlockTime:   ts,
					TxID:        tx.ID(),
					TxOrigin:    origin,
					ClauseIndex: uint32(clauseIndex),
					Sender:      tr.Sender,
					Recipient:   tr.Recipient,
					Amount:      tr.Amount,
				})
			}
		}
	}
	if !reflect.DeepEqual(eventLogs, expectedEvLogs) {
		fmt.Println("\nDiff event logs")
		fmt.Println(jsonDiff(expectedEvLogs, eventLogs))
		return errors.New("incorrect logs")
	}
	if !reflect.DeepEqual(transferLogs, expectedTrLogs) {
		fmt.Println("\nDiff transfer logs")
		fmt.Println(jsonDiff(expectedTrLogs, transferLogs))
		return errors.New("incorrect logs")
	}
	return nil
}

func jsonDiff(expected, actual interface{}) string {
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
					h := b.Header()
					queue <- func() {
						h.ID()
					}
					for _, tx := range b.Transactions() {
						tx := tx
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
