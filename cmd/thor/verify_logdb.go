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
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"gopkg.in/cheggaaa/pb.v1"
)

func verifyLogDB(ctx context.Context, endBlockNum uint32, repo *chain.Repository, logDB *logdb.LogDB) error {
	fmt.Println(">> Verifying log db <<")
	pb := pb.New64(int64(endBlockNum)).
		Set64(0).
		SetMaxWidth(90).
		Start()
	defer func() { pb.NotPrint = true }()

	const logStep = uint32(100)

	var (
		chain       = repo.NewBestChain()
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

	for i := uint32(1); i <= endBlockNum; i++ {
		b, err := chain.GetBlock(i)
		if err != nil {
			return err
		}
		id := b.Header().ID()

		if i > logLimit {
			logLimit += logStep
			evLogs, err = logDB.FilterEvents(context.TODO(), &logdb.EventFilter{
				Range: &logdb.Range{
					From: i,
					To:   logLimit,
				},
			})
			if err != nil {
				return err
			}
			trLogs, err = logDB.FilterTransfers(context.TODO(), &logdb.TransferFilter{
				Range: &logdb.Range{
					From: i,
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
	return nil
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
