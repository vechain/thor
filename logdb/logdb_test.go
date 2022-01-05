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
	"github.com/vechain/thor/block"
	logdb "github.com/vechain/thor/logdb"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
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

	for i := 0; i < 100; i++ {

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
			{"query all events", &logdb.EventFilter{}, allEvents},
			{"query all events asc", &logdb.EventFilter{Order: logdb.ASC}, allEvents},
			{"query all events desc", &logdb.EventFilter{Order: logdb.DESC}, allEvents.Reverse()},
			{"query all events limit offset", &logdb.EventFilter{Options: &logdb.Options{Offset: 1, Limit: 10}}, allEvents[1:11]},
			{"query all events range", &logdb.EventFilter{Range: &logdb.Range{From: 10, To: 20}}, allEvents.Filter(func(ev *logdb.Event) bool { return ev.BlockNumber >= 10 && ev.BlockNumber <= 20 })},
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
			{"query all transfers", &logdb.TransferFilter{}, allTransfers},
			{"query all transfers asc", &logdb.TransferFilter{Order: logdb.ASC}, allTransfers},
			{"query all transfers desc", &logdb.TransferFilter{Order: logdb.DESC}, allTransfers.Reverse()},
			{"query all transfers limit offset", &logdb.TransferFilter{Options: &logdb.Options{Offset: 1, Limit: 10}}, allTransfers[1:11]},
			{"query all transfers range", &logdb.TransferFilter{Range: &logdb.Range{From: 10, To: 20}}, allTransfers.Filter(func(tr *logdb.Transfer) bool { return tr.BlockNumber >= 10 && tr.BlockNumber <= 20 })},
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
