// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"math/big"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newTx(txType tx.Type) *tx.Transaction {
	trx := tx.NewBuilder(txType).Build()

	pk, _ := crypto.GenerateKey()

	sig, _ := crypto.Sign(trx.Hash().Bytes(), pk)
	return trx.WithSignature(sig)
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

type eventLogs []*Event

func (logs eventLogs) Filter(f func(ev *Event) bool) (ret eventLogs) {
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

type transferLogs []*Transfer

func (logs transferLogs) Filter(f func(tr *Transfer) bool) (ret transferLogs) {
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
			allEvents = append(allEvents, &Event{
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

			allTransfers = append(allTransfers, &Transfer{
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
			arg  *EventFilter
			want eventLogs
		}{
			{"query all events", &EventFilter{}, allEvents},
			{"query all events with nil option", nil, allEvents},
			{"query all events asc", &EventFilter{Order: ASC}, allEvents},
			{"query all events desc", &EventFilter{Order: DESC}, allEvents.Reverse()},
			{"query all events limit offset", &EventFilter{Options: &Options{Offset: 1, Limit: 10}}, allEvents[1:11]},
			{
				"query all events range",
				&EventFilter{Range: &Range{From: 10, To: 20}},
				allEvents.Filter(func(ev *Event) bool { return ev.BlockNumber >= 10 && ev.BlockNumber <= 20 }),
			},
			{
				"query events with range and desc",
				&EventFilter{Range: &Range{From: 10, To: 20}, Order: DESC},
				allEvents.Filter(func(ev *Event) bool { return ev.BlockNumber >= 10 && ev.BlockNumber <= 20 }).Reverse(),
			},
			{
				// An inverted range (To < From) must yield no results, not everything from From onward.
				"query events with inverted range",
				&EventFilter{Range: &Range{From: 20, To: 10}},
				nil,
			},
			{"query events with limit with desc", &EventFilter{Order: DESC, Options: &Options{Limit: 10}}, allEvents.Reverse()[0:10]},
			{
				"query all events with criteria",
				&EventFilter{CriteriaSet: []*EventCriteria{{Address: &allEvents[1].Address}}},
				allEvents.Filter(func(ev *Event) bool {
					return ev.Address == allEvents[1].Address
				}),
			},
			{
				"query all events with multi-criteria",
				&EventFilter{
					CriteriaSet: []*EventCriteria{
						{Address: &allEvents[1].Address},
						{Topics: [5]*thor.Bytes32{allEvents[2].Topics[0]}},
						{Topics: [5]*thor.Bytes32{allEvents[3].Topics[0]}},
					},
				},
				allEvents.Filter(func(ev *Event) bool {
					return ev.Address == allEvents[1].Address || *ev.Topics[0] == *allEvents[2].Topics[0] || *ev.Topics[0] == *allEvents[3].Topics[0]
				}),
			},
			{
				"query all events with multi-value multi-criteria",
				&EventFilter{
					CriteriaSet: []*EventCriteria{
						{Address: &allEvents[1].Address},
						{Address: &allEvents[2].Address, Topics: multiTopicsCriteria},
						{Topics: [5]*thor.Bytes32{allEvents[3].Topics[0]}},
					},
				},
				allEvents.Filter(func(ev *Event) bool {
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
			arg  *TransferFilter
			want transferLogs
		}{
			{"query all transfers", &TransferFilter{}, allTransfers},
			{"query all transfers with nil option", nil, allTransfers},
			{"query all transfers asc", &TransferFilter{Order: ASC}, allTransfers},
			{"query all transfers desc", &TransferFilter{Order: DESC}, allTransfers.Reverse()},
			{"query all transfers limit offset", &TransferFilter{Options: &Options{Offset: 1, Limit: 10}}, allTransfers[1:11]},
			{
				"query all transfers range",
				&TransferFilter{Range: &Range{From: 10, To: 20}},
				allTransfers.Filter(func(tr *Transfer) bool { return tr.BlockNumber >= 10 && tr.BlockNumber <= 20 }),
			},
			{
				"query transfers with range and desc",
				&TransferFilter{Range: &Range{From: 10, To: 20}, Order: DESC},
				allTransfers.Filter(func(tr *Transfer) bool { return tr.BlockNumber >= 10 && tr.BlockNumber <= 20 }).Reverse(),
			},
			{
				// An inverted range (To < From) must yield no results, not everything from From onward.
				"query transfers with inverted range",
				&TransferFilter{Range: &Range{From: 20, To: 10}},
				nil,
			},
			{"query transfers with limit with desc", &TransferFilter{Order: DESC, Options: &Options{Limit: 10}}, allTransfers.Reverse()[0:10]},
			{
				"query all transfers with criteria",
				&TransferFilter{CriteriaSet: []*TransferCriteria{{Sender: &allTransfers[1].Sender}}},
				allTransfers.Filter(func(tr *Transfer) bool {
					return tr.Sender == allTransfers[1].Sender
				}),
			},
			{
				"query all transfers with multi-criteria",
				&TransferFilter{CriteriaSet: []*TransferCriteria{{Sender: &allTransfers[1].Sender}, {Recipient: &allTransfers[2].Recipient}}},
				allTransfers.Filter(func(tr *Transfer) bool {
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

func TestAdditionalEventIndexes(t *testing.T) {
	tests := []struct {
		name                    string
		existingAdditionalIndex string
		createAdditionalIndexes bool
		wantEventI5             bool
		wantEventI6             bool
	}{
		{
			name:        "fresh database creates all additional indexes",
			wantEventI5: true,
			wantEventI6: true,
		},
		{
			name:                    "existing database does not create missing indexes when disabled",
			existingAdditionalIndex: "event_i5",
			wantEventI5:             true,
		},
		{
			name:                    "existing database creates event_i6 when event_i5 already exists",
			existingAdditionalIndex: "event_i5",
			createAdditionalIndexes: true,
			wantEventI5:             true,
			wantEventI6:             true,
		},
		{
			name:                    "existing database creates event_i5 when event_i6 already exists",
			existingAdditionalIndex: "event_i6",
			createAdditionalIndexes: true,
			wantEventI5:             true,
			wantEventI6:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "logs.db")
			if tt.existingAdditionalIndex != "" {
				seedDB, err := sql.Open("sqlite3", path)
				require.NoError(t, err)

				_, err = seedDB.Exec(refTableScheme + eventTableSchema + transferTableSchema)
				require.NoError(t, err)

				switch tt.existingAdditionalIndex {
				case "event_i5":
					_, err = seedDB.Exec("CREATE INDEX event_i5 ON event (topic4, address) WHERE topic4 IS NOT NULL")
				case "event_i6":
					_, err = seedDB.Exec("CREATE INDEX event_i6 ON event (topic0)")
				}
				require.NoError(t, err)
				require.NoError(t, seedDB.Close())
			}

			db, err := New(path, tt.createAdditionalIndexes, 4)
			require.NoError(t, err)
			defer db.Close()

			assert.Equal(t, tt.wantEventI5, indexExists(t, db.readDB, "event_i5"))
			assert.Equal(t, tt.wantEventI6, indexExists(t, db.readDB, "event_i6"))
		})
	}
}

func TestTopicOnlyQueryPlanUsesSequenceBounds(t *testing.T) {
	db, err := NewMem()
	require.NoError(t, err)
	defer db.Close()

	singleTopic := randBytes32()
	multipleTopics := []thor.Bytes32{randBytes32(), randBytes32(), randBytes32()}
	showAPITopics := []thor.Bytes32{
		thor.MustParseBytes32("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
		thor.MustParseBytes32("0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925"),
		thor.MustParseBytes32("0xe1fffcc4923d04b559f4d29a8bfc6cda04eb5b0d3c460751c2402c5c5cc9109c"),
		thor.MustParseBytes32("0x7fcf532c15f0a6db0bd6d0e038bea71d30d808c7d98cb3bf7268a95bf5081b65"),
		thor.MustParseBytes32("0xda9fa7c1b00402c17d0161b249b1ab8bbec047c5a52207b9c112deffd817036b"),
		thor.MustParseBytes32("0xc6a377bfc4eb120024a8ac08eef205be16b817020812c73223e81d1bdb9708ec"),
	}

	tests := []struct {
		name             string
		filter           *EventFilter
		wantOrderedIndex bool
	}{
		{
			name: "single topic",
			filter: &EventFilter{
				Range:       &Range{From: 100, To: 100},
				CriteriaSet: topicOnlyCriteria(singleTopic),
				Options:     &Options{Limit: 1000},
			},
			wantOrderedIndex: true,
		},
		{
			name: "multiple topics",
			filter: &EventFilter{
				Range:       &Range{From: 100, To: 100},
				CriteriaSet: topicOnlyCriteria(multipleTopics...),
				Options:     &Options{Limit: 1000},
			},
		},
		{
			name: "show-api request",
			filter: &EventFilter{
				Range:       &Range{From: 25_769_599, To: 25_769_599},
				CriteriaSet: topicOnlyCriteria(showAPITopics...),
				Order:       ASC,
				Options:     &Options{Offset: 0, Limit: 1000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args, err := buildEventFilterSubQuery(tt.filter)
			require.NoError(t, err)

			rows, err := db.readDB.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
			require.NoError(t, err)
			defer rows.Close()

			var details []string
			for rows.Next() {
				var id, parent, notUsed int
				var detail string
				require.NoError(t, rows.Scan(&id, &parent, &notUsed, &detail))
				details = append(details, detail)
			}
			require.NoError(t, rows.Err())

			plan := strings.Join(details, "\n")
			assert.Contains(t, plan, "USING COVERING INDEX event_i6")
			assert.Contains(t, plan, "rowid>?")
			assert.Contains(t, plan, "rowid<?")
			if tt.wantOrderedIndex {
				assert.NotContains(t, plan, "USE TEMP B-TREE FOR ORDER BY")
			}
		})
	}
}

func topicOnlyCriteria(topics ...thor.Bytes32) []*EventCriteria {
	criteria := make([]*EventCriteria, len(topics))
	for i := range topics {
		criteria[i] = &EventCriteria{Topics: [5]*thor.Bytes32{&topics[i]}}
	}
	return criteria
}

func indexExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='index' AND name=?)", name).Scan(&exists)
	require.NoError(t, err)
	return exists
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

func TestStatementCacheRace(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	assert.NoError(t, err)
	defer db.Close()

	cache := newStmtCache(db)
	defer cache.Clear()

	// Create a simple table for testing
	_, err = db.Exec("CREATE TABLE test (id INTEGER)")
	assert.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 100

	// Test concurrent access to the same query
	for range numGoroutines {
		wg.Go(func() {
			stmt, err := cache.Prepare("SELECT 1")
			assert.NoError(t, err)
			assert.NotNil(t, stmt)
		})
	}

	// Test concurrent access to different queries
	for i := range numGoroutines {
		wg.Go(func() {
			query := "SELECT " + string(rune('0'+i%10))
			stmt, err := cache.Prepare(query)
			assert.NoError(t, err)
			assert.NotNil(t, stmt)
		})
	}

	wg.Wait()
}
