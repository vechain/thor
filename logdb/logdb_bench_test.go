// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
	VTHO_ADDRESS = "0x0000000000000000000000000000456E65726779"
	VTHO_TOPIC   = "0xDDF252AD1BE2C89B69C2B068FC378DAA952BA7F163C4A11628F55A4DF523B3EF"
	TEST_ADDRESS = "0x7567D83B7B8D80ADDCB281A71D54FC7B3364FFED"
)

var dbPath string

// Command used to benchmark
//
// go test -bench="^Benchmark"  -benchmem -count=5 github.com/vechain/thor/v2/logdb -dbPath <path-to-logs.db> |tee -a master.txt
// go test -bench="^Benchmark"  -benchmem -count=5 github.com/vechain/thor/v2/logdb -dbPath <path-to-logs.db> |tee -a pr.txt
// benchstat maser.txt pr.txt
//

func init() {
	flag.StringVar(&dbPath, "dbPath", "", "Path to the database file")
}

// TestLogDB_NewestBlockID performs a series of read/write benchmarks on the NewestBlockID functionality of LogDB.
// It benchmarks the creating, writing, committing a new block, followed by fetching this new block as the NewestBlockID
func BenchmarkFakeDB_NewestBlockID(t *testing.B) {
	db, err := createTempDB()
	require.NoError(t, err)
	defer db.Close()

	b := new(block.Builder).
		ParentID(new(block.Builder).Build().Header().ID()).
		Transaction(newTx()).
		Build()
	receipts := tx.Receipts{newReceipt()}

	w := db.NewWriter()
	require.NoError(t, w.Write(b, receipts))
	require.NoError(t, w.Commit())

	tests := []struct {
		name    string
		prepare func() (thor.Bytes32, error)
	}{
		{
			"newest block id",
			func() (thor.Bytes32, error) {
				b = new(block.Builder).
					ParentID(b.Header().ID()).
					Build()
				receipts := tx.Receipts{newReceipt()}

				require.NoError(t, w.Write(b, receipts))
				require.NoError(t, w.Commit())

				return b.Header().ID(), nil
			},
		},
	}

	t.ResetTimer()
	for _, tt := range tests {
		t.Run(tt.name, func(b *testing.B) {
			for b.Loop() {
				want, err := tt.prepare()
				require.NoError(t, err)

				got, err := db.NewestBlockID()
				if err != nil {
					b.Fatal(err)
				}
				assert.Equal(b, want, got)
			}
		})
	}
}

// BenchmarkFakeDB_WriteBlocks creates a temporary database, performs some write + commit benchmarks and then deletes the db
func BenchmarkFakeDB_WriteBlocks(t *testing.B) {
	db, err := createTempDB()
	require.NoError(t, err)
	defer db.Close()

	blk := new(block.Builder).Build()
	w := db.NewWriter()
	writeCount := 10_000

	tests := []struct {
		name      string
		writeFunc func(b *testing.B)
	}{
		{
			"repeated writes",
			func(_ *testing.B) {
				for range writeCount {
					blk = new(block.Builder).
						ParentID(blk.Header().ID()).
						Transaction(newTx()).
						Build()
					receipts := tx.Receipts{newReceipt(), newReceipt()}
					require.NoError(t, w.Write(blk, receipts))
					require.NoError(t, w.Commit())
				}
			},
		},
		{
			"batched writes",
			func(_ *testing.B) {
				for range writeCount {
					blk = new(block.Builder).
						ParentID(blk.Header().ID()).
						Transaction(newTx()).
						Build()
					receipts := tx.Receipts{newReceipt(), newReceipt()}
					require.NoError(t, w.Write(blk, receipts))
				}
				require.NoError(t, w.Commit())
			},
		},
	}

	t.ResetTimer()
	for _, tt := range tests {
		t.Run(tt.name, func(b *testing.B) {
			for t.Loop() {
				tt.writeFunc(b)
			}
		})
	}
}

// BenchmarkTestDB_HasBlockID opens a log.db file and measures the performance of the HasBlockID functionality of LogDB.
// It uses unbounded event filtering to check for blocks existence using the HasBlockID
func BenchmarkTestDB_HasBlockID(b *testing.B) {
	db, err := loadDBFromDisk(b)
	require.NoError(b, err)
	defer db.Close()

	// find the first 500k blocks with events
	events, err := db.FilterEvents(context.Background(), &EventFilter{Options: &Options{Offset: 0, Limit: 500_000}})
	require.NoError(b, err)
	require.GreaterOrEqual(b, len(events), 500_000, "there should be more than 500k events in the db")

	for b.Loop() {
		for _, event := range events {
			has, err := db.HasBlockID(event.BlockID)
			require.NoError(b, err)
			require.True(b, has)
		}
	}
}

// BenchmarkTestDB_FilterEvents opens a log.db file and measures the performance of the Event filtering functionality of LogDB.
func BenchmarkTestDB_FilterEvents(b *testing.B) {
	db, err := loadDBFromDisk(b)
	require.NoError(b, err)
	defer db.Close()

	vthoAddress := thor.MustParseAddress(VTHO_ADDRESS)
	topic := thor.MustParseBytes32(VTHO_TOPIC)

	addressFilterCriteria := []*EventCriteria{
		{
			Address: &vthoAddress,
		},
	}
	topicFilterCriteria := []*EventCriteria{
		{
			Topics: [5]*thor.Bytes32{&topic, nil, nil, nil, nil},
		},
	}

	tests := []struct {
		name string
		arg  *EventFilter
	}{
		{"AddressCriteriaFilter", &EventFilter{CriteriaSet: addressFilterCriteria, Options: &Options{Offset: 0, Limit: 500000}}},
		{"TopicCriteriaFilter", &EventFilter{CriteriaSet: topicFilterCriteria, Options: &Options{Offset: 0, Limit: 500000}}},
		{"EventLimit", &EventFilter{Order: ASC, Options: &Options{Offset: 0, Limit: 500000}}},
		{"EventLimitDesc", &EventFilter{Order: DESC, Options: &Options{Offset: 0, Limit: 500000}}},
		{"EventRange", &EventFilter{Range: &Range{From: 500000, To: 1_000_000}}},
		{"EventRangeDesc", &EventFilter{Range: &Range{From: 500000, To: 1_000_000}, Order: DESC}},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				_, err = db.FilterEvents(context.Background(), tt.arg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkTestDB_FilterEvents opens a log.db file and measures the performance of the Transfer filtering functionality of LogDB.
// Running: go test -bench=BenchmarkTestDB_FilterTransfers  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
func BenchmarkTestDB_FilterTransfers(b *testing.B) {
	db, err := loadDBFromDisk(b)
	require.NoError(b, err)
	defer db.Close()

	txOrigin := thor.MustParseAddress(TEST_ADDRESS)
	transferCriteria := []*TransferCriteria{
		{
			TxOrigin:  &txOrigin,
			Sender:    nil,
			Recipient: nil,
		},
	}

	tests := []struct {
		name string
		arg  *TransferFilter
	}{
		{"TransferCriteria", &TransferFilter{CriteriaSet: transferCriteria, Options: &Options{Offset: 0, Limit: 500_000}}},
		{"TransferCriteriaDesc", &TransferFilter{Order: DESC, CriteriaSet: transferCriteria, Options: &Options{Offset: 0, Limit: 500_000}}},
		{"Ranged500K", &TransferFilter{Range: &Range{From: 500_000, To: 1_000_000}}},
		{"Ranged500KDesc", &TransferFilter{Range: &Range{From: 500_000, To: 1_000_000}, Order: DESC}},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for b.Loop() {
				_, err = db.FilterTransfers(context.Background(), tt.arg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func createTempDB() (*LogDB, error) {
	dir, err := os.MkdirTemp("", "tempdir-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "temp-*.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	db, err := New(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("unable to load logdb: %w", err)
	}

	return db, nil
}

func loadDBFromDisk(b *testing.B) (*LogDB, error) {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath")
	}

	return New(dbPath)
}
