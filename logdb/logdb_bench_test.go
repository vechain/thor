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
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
	VTHO_ADDRESS = "0x0000000000000000000000000000456E65726779"
	VTHO_TOPIC   = "0xDDF252AD1BE2C89B69C2B068FC378DAA952BA7F163C4A11628F55A4DF523B3EF"
	TEST_ADDRESS = "0x7567D83B7B8D80ADDCB281A71D54FC7B3364FFED"
)

var dbPath string

func init() {
	flag.StringVar(&dbPath, "dbPath", "", "Path to the database file")
}

// TestLogDB_NewestBlockID performs a series of read/write benchmarks on the NewestBlockID functionality of the LogDB.
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

	for _, tt := range tests {
		t.Run(tt.name, func(b *testing.B) {
			t.ResetTimer()
			for i := 0; i < b.N; i++ {
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

	b := new(block.Builder).Build()
	w := db.NewWriter()

	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		for i := 0; i < 10_000; i++ {
			b = new(block.Builder).
				ParentID(b.Header().ID()).
				Transaction(newTx()).
				Build()
			receipts := tx.Receipts{newReceipt(), newReceipt()}
			require.NoError(t, w.Write(b, receipts))
			require.NoError(t, w.Commit())
		}
	}
}

// BenchmarkTestDB_HasBlockID opens a log.db file and measures the performance of the HasBlockID functionality of LogDB.
// Running: go test -bench=BenchmarkTestDB_HasBlockID  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
// It uses unbounded event filtering to check for blocks existence using the HasBlockID
func BenchmarkTestDB_HasBlockID(b *testing.B) {
	db, err := loadDBFromDisk(b)
	require.NoError(b, err)
	defer db.Close()

	// find the first 500k blocks with events
	events, err := db.FilterEvents(context.Background(), &logdb.EventFilter{Options: &logdb.Options{Offset: 0, Limit: 500_000}})
	require.NoError(b, err)
	require.GreaterOrEqual(b, len(events), 500_000, "there should be more than 500k events in the db")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, event := range events {
			has, err := db.HasBlockID(event.BlockID)
			require.NoError(b, err)
			require.True(b, has)
		}
	}
}

// BenchmarkTestDB_FilterEvents opens a log.db file and measures the performance of the Event filtering functionality of LogDB.
// Running: go test -bench=BenchmarkTestDB_FilterEvents  -benchmem  github.com/vechain/thor/v2/logdb -dbPath /path/to/log.db
func BenchmarkTestDB_FilterEvents(b *testing.B) {
	db, err := loadDBFromDisk(b)
	require.NoError(b, err)
	defer db.Close()

	vthoAddress := thor.MustParseAddress(VTHO_ADDRESS)
	topic := thor.MustParseBytes32(VTHO_TOPIC)

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

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
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

func createTempDB() (*logdb.LogDB, error) {
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

	db, err := logdb.New(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("unable to load logdb: %w", err)
	}

	return db, nil
}

func loadDBFromDisk(b *testing.B) (*logdb.LogDB, error) {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath")
	}

	return logdb.New(dbPath)
}
