// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb_test

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	logdb "github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newTx() *tx.Transaction {
	tx := new(tx.Builder).Build()
	pk, _ := crypto.GenerateKey()

	sig, _ := crypto.Sign(tx.Hash().Bytes(), pk)
	return tx.WithSignature(sig)
}

func randAddress() (addr thor.Address) {
	rand.Read(addr[:])
	return
}

func randBytes32() (b thor.Bytes32) {
	rand.Read(b[:])
	return
}

func newReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: randAddress(),
					Topics:  []thor.Bytes32{randBytes32()},
					Data:    randBytes32().Bytes(),
				}},
				Transfers: tx.Transfers{{
					Sender:    randAddress(),
					Recipient: randAddress(),
					Amount:    new(big.Int).SetBytes(randAddress().Bytes()),
				}},
			},
		},
	}
}

func newEventOnlyReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: randAddress(),
					Topics:  []thor.Bytes32{randBytes32()},
					Data:    randBytes32().Bytes(),
				}},
			},
		},
	}
}

func newTransferOnlyReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Transfers: tx.Transfers{{
					Sender:    randAddress(),
					Recipient: randAddress(),
					Amount:    new(big.Int).SetBytes(randAddress().Bytes()),
				}},
			},
		},
	}
}

type eventLogs []*logdb.Event

func (logs eventLogs) Filter(f func(ev *logdb.Event) bool) (ret eventLogs) {
	for _, ev := range logs {
		if f(ev) {
			ret = append(ret, ev)
		}
	}
	return
}

func (logs eventLogs) Reverse() (ret eventLogs) {
	for i := len(logs) - 1; i >= 0; i-- {
		ret = append(ret, logs[i])
	}
	return
}

type transferLogs []*logdb.Transfer

func (logs transferLogs) Filter(f func(tr *logdb.Transfer) bool) (ret transferLogs) {
	for _, tr := range logs {
		if f(tr) {
			ret = append(ret, tr)
		}
	}
	return
}

func (logs transferLogs) Reverse() (ret transferLogs) {
	for i := len(logs) - 1; i >= 0; i-- {
		ret = append(ret, logs[i])
	}
	return
}

func TestEvents(t *testing.T) {
	db, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := new(block.Builder).Build()

	var allEvents eventLogs
	var allTransfers transferLogs

	for i := 0; i < 2000; i++ {
		b = new(block.Builder).
			ParentID(b.Header().ID()).
			Transaction(newTx()).
			Transaction(newTx()).
			Build()
		receipts := tx.Receipts{newReceipt(), newReceipt()}

		for j := 0; j < len(receipts); j++ {
			tx := b.Transactions()[j]
			receipt := receipts[j]
			origin, _ := tx.Origin()
			allEvents = append(allEvents, &logdb.Event{
				BlockNumber: b.Header().Number(),
				Index:       uint32(j),
				BlockID:     b.Header().ID(),
				BlockTime:   b.Header().Timestamp(),
				TxID:        tx.ID(),
				TxOrigin:    origin,
				ClauseIndex: 0,
				Address:     receipt.Outputs[0].Events[0].Address,
				Topics:      [5]*thor.Bytes32{&receipt.Outputs[0].Events[0].Topics[0]},
				Data:        receipt.Outputs[0].Events[0].Data,
			})

			allTransfers = append(allTransfers, &logdb.Transfer{
				BlockNumber: b.Header().Number(),
				Index:       uint32(j),
				BlockID:     b.Header().ID(),
				BlockTime:   b.Header().Timestamp(),
				TxID:        tx.ID(),
				TxOrigin:    origin,
				ClauseIndex: 0,
				Sender:      receipt.Outputs[0].Transfers[0].Sender,
				Recipient:   receipt.Outputs[0].Transfers[0].Recipient,
				Amount:      receipt.Outputs[0].Transfers[0].Amount,
			})
		}

		w := db.NewWriter()
		if err := w.Write(b, receipts); err != nil {
			t.Fatal(err)
		}

		if err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	{
		tests := []struct {
			name string
			arg  *logdb.EventFilter
			want eventLogs
		}{
			{"query all events", &logdb.EventFilter{}, allEvents[:1000]},
			{"query all events with nil option", nil, allEvents.Reverse()[:1000]},
			{"query all events asc", &logdb.EventFilter{Order: logdb.ASC}, allEvents[:1000]},
			{"query all events desc", &logdb.EventFilter{Order: logdb.DESC}, allEvents.Reverse()[:1000]},
			{"query all events limit offset", &logdb.EventFilter{Options: &logdb.Options{Offset: 1, Limit: 10}}, allEvents[1:11]},
			{"query all events outsized limit ", &logdb.EventFilter{Options: &logdb.Options{Limit: 2000}}, allEvents[:1000]},
			{"query all events outsized limit offset", &logdb.EventFilter{Options: &logdb.Options{Offset: 2, Limit: 2000}}, allEvents[2:1002]},
			{"query all events range", &logdb.EventFilter{Range: &logdb.Range{From: 10, To: 20}}, allEvents.Filter(func(ev *logdb.Event) bool { return ev.BlockNumber >= 10 && ev.BlockNumber <= 20 })},
			{"query events with range and desc", &logdb.EventFilter{Range: &logdb.Range{From: 10, To: 20}, Order: logdb.DESC}, allEvents.Filter(func(ev *logdb.Event) bool { return ev.BlockNumber >= 10 && ev.BlockNumber <= 20 }).Reverse()},
			{"query events with limit with desc", &logdb.EventFilter{Order: logdb.DESC, Options: &logdb.Options{Limit: 10}}, allEvents.Reverse()[0:10]},
			{"query all events with criteria", &logdb.EventFilter{CriteriaSet: []*logdb.EventCriteria{{Address: &allEvents[1].Address}}}, allEvents.Filter(func(ev *logdb.Event) bool {
				return ev.Address == allEvents[1].Address
			})},
			{"query all events with multi-criteria", &logdb.EventFilter{CriteriaSet: []*logdb.EventCriteria{{Address: &allEvents[1].Address}, {Topics: [5]*thor.Bytes32{allEvents[2].Topics[0]}}, {Topics: [5]*thor.Bytes32{allEvents[3].Topics[0]}}}}, allEvents.Filter(func(ev *logdb.Event) bool {
				return ev.Address == allEvents[1].Address || *ev.Topics[0] == *allEvents[2].Topics[0] || *ev.Topics[0] == *allEvents[3].Topics[0]
			})},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := db.FilterEvents(context.Background(), tt.arg)
				assert.Nil(t, err)
				assert.Equal(t, tt.want, eventLogs(got))
			})
		}
	}

	{
		tests := []struct {
			name string
			arg  *logdb.TransferFilter
			want transferLogs
		}{
			{"query all transfers", &logdb.TransferFilter{}, allTransfers[:1000]},
			{"query all transfers with nil option", nil, allTransfers.Reverse()[:1000]},
			{"query all transfers asc", &logdb.TransferFilter{Order: logdb.ASC}, allTransfers[:1000]},
			{"query all transfers desc", &logdb.TransferFilter{Order: logdb.DESC}, allTransfers.Reverse()[:1000]},
			{"query all transfers limit offset", &logdb.TransferFilter{Options: &logdb.Options{Offset: 1, Limit: 10}}, allTransfers[1:11]},
			{"query all transfers outsized limit ", &logdb.TransferFilter{Options: &logdb.Options{Limit: 2000}}, allTransfers[:1000]},
			{"query all transfers outsized limit offset", &logdb.TransferFilter{Options: &logdb.Options{Offset: 2, Limit: 2000}}, allTransfers[2:1002]},
			{"query all transfers range", &logdb.TransferFilter{Range: &logdb.Range{From: 10, To: 20}}, allTransfers.Filter(func(tr *logdb.Transfer) bool { return tr.BlockNumber >= 10 && tr.BlockNumber <= 20 })},
			{"query transfers with range and desc", &logdb.TransferFilter{Range: &logdb.Range{From: 10, To: 20}, Order: logdb.DESC}, allTransfers.Filter(func(tr *logdb.Transfer) bool { return tr.BlockNumber >= 10 && tr.BlockNumber <= 20 }).Reverse()},
			{"query transfers with limit with desc", &logdb.TransferFilter{Order: logdb.DESC, Options: &logdb.Options{Limit: 10}}, allTransfers.Reverse()[0:10]},
			{"query all transfers with criteria", &logdb.TransferFilter{CriteriaSet: []*logdb.TransferCriteria{{Sender: &allTransfers[1].Sender}}}, allTransfers.Filter(func(tr *logdb.Transfer) bool {
				return tr.Sender == allTransfers[1].Sender
			})},
			{"query all transfers with multi-criteria", &logdb.TransferFilter{CriteriaSet: []*logdb.TransferCriteria{{Sender: &allTransfers[1].Sender}, {Recipient: &allTransfers[2].Recipient}}}, allTransfers.Filter(func(tr *logdb.Transfer) bool {
				return tr.Sender == allTransfers[1].Sender || tr.Recipient == allTransfers[2].Recipient
			})},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := db.FilterTransfers(context.Background(), tt.arg)
				assert.Nil(t, err)
				assert.Equal(t, tt.want, transferLogs(got))
			})
		}
	}
}

func TestLogDB_NewestBlockID(t *testing.T) {
	db, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := new(block.Builder).Build()

	b = new(block.Builder).
		ParentID(b.Header().ID()).
		Transaction(newTx()).
		Build()
	receipts := tx.Receipts{newReceipt()}

	w := db.NewWriter()
	if err := w.Write(b, receipts); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		prepare func() (thor.Bytes32, error)
	}{
		{
			"newest block id",
			func() (thor.Bytes32, error) {
				return b.Header().ID(), nil
			},
		}, {
			"add empty block, best should remain unchanged",
			func() (thor.Bytes32, error) {
				wanted := b.Header().ID()
				b = new(block.Builder).ParentID(b.Header().ID()).Build()
				receipts = tx.Receipts{}

				w := db.NewWriter()
				if err := w.Write(b, receipts); err != nil {
					return thor.Bytes32{}, nil
				}
				if err := w.Commit(); err != nil {
					return thor.Bytes32{}, nil
				}
				return wanted, nil
			},
		},
		{
			"add both event and transfer, best should change",
			func() (thor.Bytes32, error) {
				b = new(block.Builder).
					ParentID(b.Header().ID()).
					Transaction(newTx()).
					Build()
				receipts := tx.Receipts{newReceipt()}

				w := db.NewWriter()
				if err := w.Write(b, receipts); err != nil {
					return thor.Bytes32{}, nil
				}
				if err := w.Commit(); err != nil {
					return thor.Bytes32{}, nil
				}
				return b.Header().ID(), nil
			},
		},
		{
			"add event only, best should change",
			func() (thor.Bytes32, error) {
				b = new(block.Builder).
					ParentID(b.Header().ID()).
					Transaction(newTx()).
					Build()
				receipts := tx.Receipts{newEventOnlyReceipt()}

				w := db.NewWriter()
				if err := w.Write(b, receipts); err != nil {
					return thor.Bytes32{}, nil
				}
				if err := w.Commit(); err != nil {
					return thor.Bytes32{}, nil
				}
				return b.Header().ID(), nil
			},
		},
		{
			"add transfer only, best should change",
			func() (thor.Bytes32, error) {
				b = new(block.Builder).
					ParentID(b.Header().ID()).
					Transaction(newTx()).
					Build()
				receipts := tx.Receipts{newTransferOnlyReceipt()}

				w := db.NewWriter()
				if err := w.Write(b, receipts); err != nil {
					return thor.Bytes32{}, nil
				}
				if err := w.Commit(); err != nil {
					return thor.Bytes32{}, nil
				}
				return b.Header().ID(), nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := tt.prepare()
			if err != nil {
				t.Fatal(err)
			}
			got, err := db.NewestBlockID()
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestLogDB_HasBlockID(t *testing.T) {
	db, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b0 := new(block.Builder).Build()

	b := new(block.Builder).
		ParentID(b0.Header().ID()).
		Transaction(newTx()).
		Build()
	b1 := b.Header().ID()
	receipts := tx.Receipts{newReceipt()}

	w := db.NewWriter()
	_ = w.Write(b, receipts)

	b = new(block.Builder).
		ParentID(b1).
		Build()
	b2 := b.Header().ID()
	receipts = tx.Receipts{}
	_ = w.Write(b, receipts)

	b = new(block.Builder).
		ParentID(b2).
		Transaction(newTx()).
		Build()
	b3 := b.Header().ID()
	receipts = tx.Receipts{newEventOnlyReceipt()}
	_ = w.Write(b, receipts)

	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}

	has, err := db.HasBlockID(b0.Header().ID())
	if err != nil {
		t.Fatal(err)
	}
	assert.False(t, has)

	has, err = db.HasBlockID(b1)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, has)

	has, err = db.HasBlockID(b2)
	if err != nil {
		t.Fatal(err)
	}
	assert.False(t, has)

	has, err = db.HasBlockID(b3)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, has)
}
