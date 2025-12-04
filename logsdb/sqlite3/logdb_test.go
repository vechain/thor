// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package sqlite3

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newTx(txType tx.Type) *tx.Transaction {
	trx := tx.NewBuilder(txType).Build()

	pk, _ := crypto.GenerateKey()

	sig, _ := crypto.Sign(trx.Hash().Bytes(), pk)
	return trx.WithSignature(sig)
}

func newReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: datagen.RandAddress(),
					Topics:  []thor.Bytes32{datagen.RandomHash()},
					Data:    datagen.RandomHash().Bytes(),
				}},
				Transfers: tx.Transfers{{
					Sender:    datagen.RandAddress(),
					Recipient: datagen.RandAddress(),
					Amount:    new(big.Int).SetBytes(datagen.RandomHash().Bytes()),
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
					Address: datagen.RandAddress(),
					Topics:  []thor.Bytes32{datagen.RandomHash()},
					Data:    datagen.RandomHash().Bytes(),
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
					Sender:    datagen.RandAddress(),
					Recipient: datagen.RandAddress(),
					Amount:    new(big.Int).SetBytes(datagen.RandAddress().Bytes()),
				}},
			},
		},
	}
}

type eventLogs []*logsdb.Event

func (logs eventLogs) Filter(f func(ev *logsdb.Event) bool) (ret eventLogs) {
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

type transferLogs []*logsdb.Transfer

func (logs transferLogs) Filter(f func(tr *logsdb.Transfer) bool) (ret transferLogs) {
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
	db, err := NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := new(block.Builder).Build()

	var allEvents eventLogs
	var allTransfers transferLogs

	for range 100 {
		b = new(block.Builder).
			ParentID(b.Header().ID()).
			Transaction(newTx(tx.TypeLegacy)).
			Transaction(newTx(tx.TypeDynamicFee)).
			Build()
		receipts := tx.Receipts{newReceipt(), newReceipt()}
		logIndex := 0
		transferIndex := 0
		for j, receipt := range receipts {
			tx := b.Transactions()[j]
			origin, _ := tx.Origin()
			allEvents = append(allEvents, &logsdb.Event{
				BlockNumber: b.Header().Number(),
				LogIndex:    uint32(logIndex),
				TxIndex:     uint32(j),
				BlockID:     b.Header().ID(),
				BlockTime:   b.Header().Timestamp(),
				TxID:        tx.ID(),
				TxOrigin:    origin,
				ClauseIndex: 0,
				Address:     receipt.Outputs[0].Events[0].Address,
				Topics:      [5]*thor.Bytes32{&receipt.Outputs[0].Events[0].Topics[0]},
				Data:        receipt.Outputs[0].Events[0].Data,
			})
			logIndex++

			allTransfers = append(allTransfers, &logsdb.Transfer{
				BlockNumber: b.Header().Number(),
				LogIndex:    uint32(transferIndex),
				TxIndex:     uint32(j),
				BlockID:     b.Header().ID(),
				BlockTime:   b.Header().Timestamp(),
				TxID:        tx.ID(),
				TxOrigin:    origin,
				ClauseIndex: 0,
				Sender:      receipt.Outputs[0].Transfers[0].Sender,
				Recipient:   receipt.Outputs[0].Transfers[0].Recipient,
				Amount:      receipt.Outputs[0].Transfers[0].Amount,
			})
			transferIndex++
		}

		w := db.NewWriter()
		if err := w.Write(b, receipts); err != nil {
			t.Fatal(err)
		}

		if err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	multiTopicsCriteria := [5]*thor.Bytes32{
		allEvents[2].Topics[0],
		allEvents[4].Topics[0],
	}

	{
		tests := []struct {
			name string
			arg  *logsdb.EventFilter
			want eventLogs
		}{
			{"query all events", &logsdb.EventFilter{}, allEvents},
			{"query all events with nil option", nil, allEvents},
			{"query all events asc", &logsdb.EventFilter{Order: logsdb.ASC}, allEvents},
			{"query all events desc", &logsdb.EventFilter{Order: logsdb.DESC}, allEvents.Reverse()},
			{"query all events limit offset", &logsdb.EventFilter{Options: &logsdb.Options{Offset: 1, Limit: 10}}, allEvents[1:11]},
			{
				"query all events range",
				&logsdb.EventFilter{Range: &logsdb.Range{From: 10, To: 20}},
				allEvents.Filter(func(ev *logsdb.Event) bool { return ev.BlockNumber >= 10 && ev.BlockNumber <= 20 }),
			},
			{
				"query events with range and desc",
				&logsdb.EventFilter{Range: &logsdb.Range{From: 10, To: 20}, Order: logsdb.DESC},
				allEvents.Filter(func(ev *logsdb.Event) bool { return ev.BlockNumber >= 10 && ev.BlockNumber <= 20 }).Reverse(),
			},
			{"query events with limit with desc", &logsdb.EventFilter{Order: logsdb.DESC, Options: &logsdb.Options{Limit: 10}}, allEvents.Reverse()[0:10]},
			{
				"query all events with criteria",
				&logsdb.EventFilter{CriteriaSet: []*logsdb.EventCriteria{{Address: &allEvents[1].Address}}},
				allEvents.Filter(func(ev *logsdb.Event) bool {
					return ev.Address == allEvents[1].Address
				}),
			},
			{
				"query all events with multi-criteria",
				&logsdb.EventFilter{
					CriteriaSet: []*logsdb.EventCriteria{
						{Address: &allEvents[1].Address},
						{Topics: [5]*thor.Bytes32{allEvents[2].Topics[0]}},
						{Topics: [5]*thor.Bytes32{allEvents[3].Topics[0]}},
					},
				},
				allEvents.Filter(func(ev *logsdb.Event) bool {
					return ev.Address == allEvents[1].Address || *ev.Topics[0] == *allEvents[2].Topics[0] || *ev.Topics[0] == *allEvents[3].Topics[0]
				}),
			},
			{
				"query all events with multi-value multi-criteria",
				&logsdb.EventFilter{
					CriteriaSet: []*logsdb.EventCriteria{
						{Address: &allEvents[1].Address},
						{Address: &allEvents[2].Address, Topics: multiTopicsCriteria},
						{Topics: [5]*thor.Bytes32{allEvents[3].Topics[0]}},
					},
				},
				allEvents.Filter(func(ev *logsdb.Event) bool {
					return ev.Address == allEvents[1].Address || *ev.Topics[0] == *allEvents[3].Topics[0]
				}),
			},
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
			arg  *logsdb.TransferFilter
			want transferLogs
		}{
			{"query all transfers", &logsdb.TransferFilter{}, allTransfers},
			{"query all transfers with nil option", nil, allTransfers},
			{"query all transfers asc", &logsdb.TransferFilter{Order: logsdb.ASC}, allTransfers},
			{"query all transfers desc", &logsdb.TransferFilter{Order: logsdb.DESC}, allTransfers.Reverse()},
			{"query all transfers limit offset", &logsdb.TransferFilter{Options: &logsdb.Options{Offset: 1, Limit: 10}}, allTransfers[1:11]},
			{
				"query all transfers range",
				&logsdb.TransferFilter{Range: &logsdb.Range{From: 10, To: 20}},
				allTransfers.Filter(func(tr *logsdb.Transfer) bool { return tr.BlockNumber >= 10 && tr.BlockNumber <= 20 }),
			},
			{
				"query transfers with range and desc",
				&logsdb.TransferFilter{Range: &logsdb.Range{From: 10, To: 20}, Order: logsdb.DESC},
				allTransfers.Filter(func(tr *logsdb.Transfer) bool { return tr.BlockNumber >= 10 && tr.BlockNumber <= 20 }).Reverse(),
			},
			{
				"query transfers with limit with desc",
				&logsdb.TransferFilter{Order: logsdb.DESC, Options: &logsdb.Options{Limit: 10}},
				allTransfers.Reverse()[0:10],
			},
			{
				"query all transfers with criteria",
				&logsdb.TransferFilter{CriteriaSet: []*logsdb.TransferCriteria{{Sender: &allTransfers[1].Sender}}},
				allTransfers.Filter(func(tr *logsdb.Transfer) bool {
					return tr.Sender == allTransfers[1].Sender
				}),
			},
			{
				"query all transfers with multi-criteria",
				&logsdb.TransferFilter{CriteriaSet: []*logsdb.TransferCriteria{{Sender: &allTransfers[1].Sender}, {Recipient: &allTransfers[2].Recipient}}},
				allTransfers.Filter(func(tr *logsdb.Transfer) bool {
					return tr.Sender == allTransfers[1].Sender || tr.Recipient == allTransfers[2].Recipient
				}),
			},
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

// TestLogDB_NewestBlockID performs a series of read/write tests on the NewestBlockID functionality of the
// It validates the correctness of the NewestBlockID method under various scenarios.
func TestLogDB_NewestBlockID(t *testing.T) {
	db, err := NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := new(block.Builder).Build()

	b = new(block.Builder).
		ParentID(b.Header().ID()).
		Transaction(newTx(tx.TypeLegacy)).
		Transaction(newTx(tx.TypeDynamicFee)).
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
					Transaction(newTx(tx.TypeLegacy)).
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
					Transaction(newTx(tx.TypeLegacy)).
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
					Transaction(newTx(tx.TypeLegacy)).
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

// TestLogDB_HasBlockID performs a series of tests on the HasBlockID functionality of the
func TestLogDB_HasBlockID(t *testing.T) {
	db, err := NewMem()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b0 := new(block.Builder).Build()

	b := new(block.Builder).
		ParentID(b0.Header().ID()).
		Transaction(newTx(tx.TypeLegacy)).
		Transaction(newTx(tx.TypeDynamicFee)).
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
		Transaction(newTx(tx.TypeLegacy)).
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

func TestRemoveLeadingZeros(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			"should remove leading zeros",
			common.Hex2Bytes("0000000000000000000000006d95e6dca01d109882fe1726a2fb9865fa41e7aa"),
			common.Hex2Bytes("6d95e6dca01d109882fe1726a2fb9865fa41e7aa"),
		},
		{
			"should not remove any bytes",
			common.Hex2Bytes("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
			common.Hex2Bytes("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
		},
		{
			"should have at least 1 byte",
			common.Hex2Bytes("00000000000000000"),
			[]byte{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeLeadingZeros(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
