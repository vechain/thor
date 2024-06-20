// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb_test

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	logdb "github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var dbPath string

func init() {
	flag.StringVar(&dbPath, "dbPath", "", "Path to the database file")
}

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
			{"query all events with nil option", nil, allEvents},
			{"query all events asc", &logdb.EventFilter{Order: logdb.ASC}, allEvents},
			{"query all events desc", &logdb.EventFilter{Order: logdb.DESC}, allEvents.Reverse()},
			{"query all events limit offset", &logdb.EventFilter{Options: &logdb.Options{Offset: 1, Limit: 10}}, allEvents[1:11]},
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
			{"query all transfers", &logdb.TransferFilter{}, allTransfers},
			{"query all transfers with nil option", nil, allTransfers},
			{"query all transfers asc", &logdb.TransferFilter{Order: logdb.ASC}, allTransfers},
			{"query all transfers desc", &logdb.TransferFilter{Order: logdb.DESC}, allTransfers.Reverse()},
			{"query all transfers limit offset", &logdb.TransferFilter{Options: &logdb.Options{Offset: 1, Limit: 10}}, allTransfers[1:11]},
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

func CreateTempDBPath() (string, error) {
	dir, err := os.MkdirTemp("", "tempdir-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "temp-*.db")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

func BenchmarkFakeDB_NewestBlockID(t *testing.B) {
	dbPath, err := CreateTempDBPath()
	defer os.Remove(dbPath)

	if err != nil {
		t.Fatal(err)
	}

	db, err := logdb.New(dbPath)
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

	t.ResetTimer()
	for _, tt := range tests {
		t.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				want, err := tt.prepare()
				if err != nil {
					b.Fatal(err)
				}
				got, err := db.NewestBlockID()
				if err != nil {
					b.Fatal(err)
				}
				assert.Equal(b, want, got)
			}
		})
	}
}

func BenchmarkFakeDB_WriteBlocks(t *testing.B) {
	dbPath, err := CreateTempDBPath()
	defer os.Remove(dbPath)

	if err != nil {
		t.Fatal(err)
	}

	db, err := logdb.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := new(block.Builder).Build()

	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for i := 0; i < 100000; i++ {
			b = new(block.Builder).
				ParentID(b.Header().ID()).
				Transaction(newTx()).
				Transaction(newTx()).
				Build()
			receipts := tx.Receipts{newReceipt(), newReceipt()}

			w := db.NewWriter()
			if err := w.Write(b, receipts); err != nil {
				t.Fatal(err)
			}

			if err := w.Commit(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// go test -bench=BenchmarkTestDB_HasBlockID  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
func BenchmarkTestDB_HasBlockID(b *testing.B) {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath")
	}
	db, err := logdb.New(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b0 := new(block.Builder).Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		has, err := db.HasBlockID(b0.Header().ID())
		if err != nil {
			b.Fatal(err)
		}
		assert.False(b, has)
	}
}

// go test -bench=BenchmarkTestDB_NewestBlockID  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
func BenchmarkTestDB_NewestBlockID(b *testing.B) {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath")
	}
	db, err := logdb.New(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.NewestBlockID()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// go test -bench=BenchmarkTestDB_FilterEvents  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
func BenchmarkTestDB_FilterEvents(b *testing.B) {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath")
	}
	db, err := logdb.New(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	eventCriteria1 := make([]*logdb.EventCriteria, 0, 1)
	eventCriteria2 := make([]*logdb.EventCriteria, 0, 1)

	addressCriteria := &logdb.EventCriteria{
		Address: &thor.Address{0x1},
	}

	topics := [5]*thor.Bytes32{new(thor.Bytes32), nil, nil, nil, nil}
	*topics[0] = thor.Bytes32{20}

	topicCriteria := &logdb.EventCriteria{
		Topics: topics,
	}

	eventCriteria1 = append(eventCriteria1, addressCriteria)
	eventCriteria2 = append(eventCriteria2, topicCriteria)

	tests := []struct {
		arg *logdb.EventFilter
	}{
		{&logdb.EventFilter{CriteriaSet: eventCriteria1, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{&logdb.EventFilter{CriteriaSet: eventCriteria2, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{&logdb.EventFilter{Order: logdb.ASC, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{&logdb.EventFilter{Order: logdb.DESC, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{&logdb.EventFilter{Options: &logdb.Options{Offset: 1, Limit: 10}}},
		{&logdb.EventFilter{Range: &logdb.Range{From: 10, To: 20}}},
		{&logdb.EventFilter{Range: &logdb.Range{From: 10, To: 20}, Order: logdb.DESC}},
		{&logdb.EventFilter{Order: logdb.DESC, Options: &logdb.Options{Limit: 10}}},
	}

	b.ResetTimer()
	for _, tt := range tests {
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), tt.arg)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// go test -bench=BenchmarkTestDB_FilterTransfers  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
func BenchmarkTestDB_FilterTransfers(b *testing.B) {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath")
	}
	db, err := logdb.New(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	transferCriteria := make([]*logdb.TransferCriteria, 0, 1)

	newCriteria := &logdb.TransferCriteria{
		TxOrigin:  &thor.Address{0x1},
		Sender:    nil,
		Recipient: nil,
	}

	transferCriteria = append(transferCriteria, newCriteria)

	tests := []struct {
		arg *logdb.TransferFilter
	}{
		{&logdb.TransferFilter{CriteriaSet: transferCriteria, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{&logdb.TransferFilter{}},
		{nil},
		{&logdb.TransferFilter{Order: logdb.ASC}},
		{&logdb.TransferFilter{Order: logdb.DESC}},
		{&logdb.TransferFilter{Options: &logdb.Options{Offset: 1, Limit: 10}}},
		{&logdb.TransferFilter{Range: &logdb.Range{From: 10, To: 20}}},
		{&logdb.TransferFilter{Range: &logdb.Range{From: 10, To: 20}, Order: logdb.DESC}},
		{&logdb.TransferFilter{Order: logdb.DESC, Options: &logdb.Options{Limit: 10}}},
	}

	b.ResetTimer()
	for _, tt := range tests {
		for i := 0; i < b.N; i++ {
			_, err := db.FilterTransfers(context.Background(), tt.arg)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
