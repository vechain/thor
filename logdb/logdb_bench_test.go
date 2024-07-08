// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	logdb "github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const VTHO_ADDRESS = "0x0000000000000000000000000000456E65726779"

var dbPath string

func init() {
	flag.StringVar(&dbPath, "dbPath", "", "Path to the database file")
}

func createTempDBPath() (string, error) {
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

// TestLogDB_NewestBlockID performs a series of read/write benchmarks on the NewestBlockID functionality of the LogDB.
// It validates the correctness of the NewestBlockID method under various scenarios.
func BenchmarkFakeDB_NewestBlockID(t *testing.B) {
	dbPath, err := createTempDBPath()
	defer os.Remove(dbPath)

	if err != nil {
		t.Fatal(err)
	}

	db, err := logdb.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := new(block.Builder).
		ParentID(new(block.Builder).Build().Header().ID()).
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

// BenchmarkFakeDB_WriteBlocks creates a temporary database, performs some write only benchmarks and then deletes it
func BenchmarkFakeDB_WriteBlocks(t *testing.B) {
	dbPath, err := createTempDBPath()
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
		for i := 0; i < 1_000; i++ {
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

// BenchmarkTestDB_HasBlockID opens a log.db file and measures the performance of the HasBlockID functionality of LogDB.
// Running: go test -bench=BenchmarkTestDB_HasBlockID  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
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

// BenchmarkTestDB_NewestBlockID opens a log.db file and measures the performance of the NewestBlockID functionality of LogDB.
// Running: go test -bench=BenchmarkTestDB_NewestBlockID  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
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

// BenchmarkTestDB_FilterEvents opens a log.db file and measures the performance of the Event filtering functionality of LogDB.
// Running: go test -bench=BenchmarkTestDB_FilterEvents  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
func BenchmarkTestDB_FilterEvents(b *testing.B) {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath")
	}
	db, err := logdb.New(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	vthoAddress := thor.MustParseAddress(VTHO_ADDRESS)
	topic := thor.MustParseBytes32("0xDDF252AD1BE2C89B69C2B068FC378DAA952BA7F163C4A11628F55A4DF523B3EF")

	addressFilterCriteria := []*logdb.EventCriteria{
		{
			Address: &vthoAddress,
		},
	}
	topicFilterCriteria := []*logdb.EventCriteria{
		{
			Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil},
		},
	}

	tests := []struct {
		name string
		arg  *logdb.EventFilter
	}{
		{"AddressCriteriaFilter", &logdb.EventFilter{CriteriaSet: addressFilterCriteria, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{"TopicCriteriaFilter", &logdb.EventFilter{CriteriaSet: topicFilterCriteria, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{"EventLimit", &logdb.EventFilter{Order: logdb.ASC, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{"EventLimitDesc", &logdb.EventFilter{Order: logdb.DESC, Options: &logdb.Options{Offset: 0, Limit: 500000}}},
		{"EventRange", &logdb.EventFilter{Range: &logdb.Range{From: 500000, To: 1_000_000}}},
		{"EventRangeDesc", &logdb.EventFilter{Range: &logdb.Range{From: 500000, To: 1_000_000}, Order: logdb.DESC}},
	}

	for j, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			var events []*logdb.Event
			for i := 0; i < b.N; i++ {
				events, err = db.FilterEvents(context.Background(), tt.arg)
				if err != nil {
					b.Fatal(err)
				}
			}
			fmt.Printf("Test %d - Total events: %d\n", j, len(events))
		})

	}
}

// BenchmarkTestDB_FilterEvents opens a log.db file and measures the performance of the Transfer filtering functionality of LogDB.
// Running: go test -bench=BenchmarkTestDB_FilterTransfers  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
func BenchmarkTestDB_FilterTransfers(b *testing.B) {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath")
	}
	db, err := logdb.New(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	txOrigin := thor.MustParseAddress("0x7567D83B7B8D80ADDCB281A71D54FC7B3364FFED")
	transferCriteria := []*logdb.TransferCriteria{
		{
			TxOrigin:  &txOrigin,
			Sender:    nil,
			Recipient: nil,
		},
	}

	tests := []struct {
		name string
		arg  *logdb.TransferFilter
	}{
		{"TransferCriteria", &logdb.TransferFilter{CriteriaSet: transferCriteria, Options: &logdb.Options{Offset: 0, Limit: 500_000}}},
		{"TransferCriteriaDesc", &logdb.TransferFilter{Order: logdb.DESC, CriteriaSet: transferCriteria, Options: &logdb.Options{Offset: 0, Limit: 500_000}}},
		{"Ranged500K", &logdb.TransferFilter{Range: &logdb.Range{From: 500_000, To: 1_000_000}}},
		{"Ranged500KDesc", &logdb.TransferFilter{Range: &logdb.Range{From: 500_000, To: 1_000_000}, Order: logdb.DESC}},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err = db.FilterTransfers(context.Background(), tt.arg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
