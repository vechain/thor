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

func verifyLogDB(ctx context.Context, endBlockNum uint32, chain *chain.Chain, logDB *logdb.LogDB) error {
	fmt.Println(">> Verifying log db <<")
	pb := pb.New64(int64(endBlockNum)).
		Set64(0).
		SetMaxWidth(90).
		Start()
	defer func() { pb.NotPrint = true }()

	from := uint32(1)
	for from < endBlockNum {
		to := from + 255
		if to > endBlockNum {
			to = endBlockNum
		}

		blocks, err := fastReadBlocks(chain, from, to)
		if err != nil {
			return err
		}
		for _, b := range blocks {
			n := b.Header().Number()
			evLogs, err := logDB.FilterEvents(context.TODO(), &logdb.EventFilter{
				Range: &logdb.Range{
					Unit: "block",
					From: uint64(n),
					To:   uint64(n),
				},
			})
			if err != nil {
				return err
			}
			trLogs, err := logDB.FilterTransfers(context.TODO(), &logdb.TransferFilter{
				Range: &logdb.Range{
					Unit: "block",
					From: uint64(n),
					To:   uint64(n),
				},
			})
			if err != nil {
				return err
			}

			receipts, err := chain.GetBlockReceipts(b.Header().ID())
			if err != nil {
				return err
			}
			if err := verifyLogDBPerBlock(b, receipts, evLogs, trLogs); err != nil {
				return err
			}
			pb.Add64(1)
		}
		from = to + 1
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
