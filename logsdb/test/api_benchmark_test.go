// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"context"
	"sync"
	"testing"

	"github.com/vechain/thor/v2/logsdb"
	pebbledb "github.com/vechain/thor/v2/logsdb/pebbledb"
	"github.com/vechain/thor/v2/logsdb/sqlite3"
	"github.com/vechain/thor/v2/thor"
)

// Global benchmark discovery data with sync.Once
var (
	benchmarkDiscoveryOnce sync.Once
	benchmarkDiscoveryData *DiscoveryData
)

// getBenchmarkDiscoveryData returns discovery data, loading it once with sync.Once
func getBenchmarkDiscoveryData() *DiscoveryData {
	benchmarkDiscoveryOnce.Do(func() {
		benchmarkDiscoveryData = GetDiscoveryData()
	})
	return benchmarkDiscoveryData
}

// parseAddressSafe safely parses an address string, returning error instead of panicking
func parseAddressSafe(s string) (thor.Address, error) {
	return thor.ParseAddress(s)
}

// parseBytes32Safe safely parses a bytes32 string, returning error instead of panicking
func parseBytes32Safe(s string) (thor.Bytes32, error) {
	return thor.ParseBytes32(s)
}

// loadSQLiteForBenchmark loads SQLite database for benchmarking
func loadSQLiteForBenchmark(b *testing.B) logsdb.LogsDB {
	if dbPath == "" {
		b.Fatal("Please provide a dbPath flag")
	}

	db, err := sqlite3.New(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	return db
}

// loadPebbleForBenchmark loads Pebble database for benchmarking
func loadPebbleForBenchmark(b *testing.B) logsdb.LogsDB {
	if pebblePath == "" {
		b.Fatal("Please provide a pebblePath flag")
	}

	db, err := pebbledb.Open(pebblePath)
	if err != nil {
		b.Fatal(err)
	}
	return db
}

// BenchmarkAPI_Events_AddressOnly_Hot benchmarks hot address queries
func BenchmarkAPI_Events_AddressOnly_Hot(b *testing.B) {
	data := getBenchmarkDiscoveryData()
	if len(data.HotAddresses) == 0 {
		b.Skip("No hot addresses available in discovery data")
	}

	// Parse address from discovery data
	hotAddr, err := parseAddressSafe(data.HotAddresses[0])
	if err != nil {
		b.Skipf("Invalid hot address format: %v", err)
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Address: &hotAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Address: &hotAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Events_AddressOnly_Sparse benchmarks sparse address queries
func BenchmarkAPI_Events_AddressOnly_Sparse(b *testing.B) {
	data := getBenchmarkDiscoveryData()
	if len(data.SparseAddresses) == 0 {
		b.Skip("No sparse addresses available in discovery data")
	}

	// Parse address from discovery data
	sparseAddr, err := parseAddressSafe(data.SparseAddresses[0])
	if err != nil {
		b.Skipf("Invalid sparse address format: %v", err)
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Address: &sparseAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Address: &sparseAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Events_TopicOnly_Hot benchmarks hot topic queries
func BenchmarkAPI_Events_TopicOnly_Hot(b *testing.B) {
	data := getBenchmarkDiscoveryData()
	if len(data.HotTopics) == 0 {
		b.Skip("No hot topics available in discovery data")
	}

	// Parse topic from discovery data and use in topic0
	hotTopic, err := parseBytes32Safe(data.HotTopics[0])
	if err != nil {
		b.Skipf("Invalid hot topic format: %v", err)
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Topics: [5]*thor.Bytes32{&hotTopic, nil, nil, nil, nil},
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Topics: [5]*thor.Bytes32{&hotTopic, nil, nil, nil, nil},
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Events_MultiTopic_TwoTopics benchmarks two-topic AND queries
func BenchmarkAPI_Events_MultiTopic_TwoTopics(b *testing.B) {
	data := getBenchmarkDiscoveryData()
	if len(data.MultiTopicPatterns) == 0 {
		b.Skip("No multi-topic patterns available in discovery data")
	}

	// Find a pattern with both topic0 and topic1
	var pattern *EventPattern
	for i := range data.MultiTopicPatterns {
		p := &data.MultiTopicPatterns[i]
		if p.Topic0 != "" && p.Topic1 != "" {
			pattern = p
			break
		}
	}

	if pattern == nil {
		b.Skip("No two-topic patterns found in discovery data")
	}

	// Parse topics from pattern
	topic0, err := parseBytes32Safe(pattern.Topic0)
	if err != nil {
		b.Skipf("Invalid topic0 format: %v", err)
	}
	topic1, err := parseBytes32Safe(pattern.Topic1)
	if err != nil {
		b.Skipf("Invalid topic1 format: %v", err)
	}

	// Parse address if available
	var addr *thor.Address
	if pattern.Address != "" {
		parsed, err := parseAddressSafe(pattern.Address)
		if err != nil {
			b.Skipf("Invalid pattern address format: %v", err)
		}
		addr = &parsed
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Address: addr,
				Topics:  [5]*thor.Bytes32{&topic0, &topic1, nil, nil, nil},
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Address: addr,
				Topics:  [5]*thor.Bytes32{&topic0, &topic1, nil, nil, nil},
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Events_Range_Large benchmarks large block range queries
func BenchmarkAPI_Events_Range_Large(b *testing.B) {
	// Use a large realistic block range (50k blocks)
	fromBlock := uint32(500000)
	toBlock := uint32(550000)

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			Range: &logsdb.Range{
				From: fromBlock,
				To:   toBlock,
			},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			Range: &logsdb.Range{
				From: fromBlock,
				To:   toBlock,
			},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Events_Range_Narrow benchmarks narrow block range queries
func BenchmarkAPI_Events_Range_Narrow(b *testing.B) {
	// Use a narrow realistic block range (100 blocks)
	fromBlock := uint32(1000000)
	toBlock := uint32(1000100)

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			Range: &logsdb.Range{
				From: fromBlock,
				To:   toBlock,
			},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			Range: &logsdb.Range{
				From: fromBlock,
				To:   toBlock,
			},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Events_LimitMedium benchmarks medium limit/offset queries
func BenchmarkAPI_Events_LimitMedium(b *testing.B) {
	data := getBenchmarkDiscoveryData()
	if len(data.HotAddresses) == 0 {
		b.Skip("No hot addresses available for limit benchmark")
	}

	// Use hot address to ensure sufficient results for offset
	hotAddr, err := parseAddressSafe(data.HotAddresses[0])
	if err != nil {
		b.Skipf("Invalid hot address format: %v", err)
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Address: &hotAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 20,
				Limit:  50,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{{
				Address: &hotAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 20,
				Limit:  50,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Events_MultiClause_OR benchmarks OR criteria queries
func BenchmarkAPI_Events_MultiClause_OR(b *testing.B) {
	data := getBenchmarkDiscoveryData()
	if len(data.HotAddresses) < 2 {
		b.Skip("Need at least 2 hot addresses for OR benchmark")
	}

	// Use two different addresses for OR semantics
	addr1, err := parseAddressSafe(data.HotAddresses[0])
	if err != nil {
		b.Skipf("Invalid first address format: %v", err)
	}
	addr2, err := parseAddressSafe(data.HotAddresses[1])
	if err != nil {
		b.Skipf("Invalid second address format: %v", err)
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{
				{Address: &addr1},
				{Address: &addr2},
			},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.EventFilter{
			CriteriaSet: []*logsdb.EventCriteria{
				{Address: &addr1},
				{Address: &addr2},
			},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterEvents(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Transfers_SenderOnly benchmarks sender-only transfer queries
func BenchmarkAPI_Transfers_SenderOnly(b *testing.B) {
	data := getBenchmarkDiscoveryData()
	if len(data.TransferHotAddresses) == 0 {
		b.Skip("No hot transfer addresses available in discovery data")
	}

	// Parse sender address from discovery data
	senderAddr, err := parseAddressSafe(data.TransferHotAddresses[0])
	if err != nil {
		b.Skipf("Invalid transfer sender address format: %v", err)
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.TransferFilter{
			CriteriaSet: []*logsdb.TransferCriteria{{
				Sender: &senderAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterTransfers(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.TransferFilter{
			CriteriaSet: []*logsdb.TransferCriteria{{
				Sender: &senderAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterTransfers(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Transfers_RecipientOnly benchmarks recipient-only transfer queries
func BenchmarkAPI_Transfers_RecipientOnly(b *testing.B) {
	data := getBenchmarkDiscoveryData()
	if len(data.TransferHotAddresses) == 0 {
		b.Skip("No hot transfer addresses available in discovery data")
	}

	// Parse recipient address from discovery data
	recipientAddr, err := parseAddressSafe(data.TransferHotAddresses[0])
	if err != nil {
		b.Skipf("Invalid transfer recipient address format: %v", err)
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.TransferFilter{
			CriteriaSet: []*logsdb.TransferCriteria{{
				Recipient: &recipientAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterTransfers(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.TransferFilter{
			CriteriaSet: []*logsdb.TransferCriteria{{
				Recipient: &recipientAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterTransfers(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAPI_Transfers_Range benchmarks transfer range queries
func BenchmarkAPI_Transfers_Range(b *testing.B) {
	data := getBenchmarkDiscoveryData()

	// Use a realistic block range for transfers
	fromBlock := uint32(800000)
	toBlock := uint32(900000)

	// Optionally combine with a sender if available
	var senderAddr *thor.Address
	if len(data.TransferHotAddresses) > 0 {
		parsed, err := parseAddressSafe(data.TransferHotAddresses[0])
		if err != nil {
			b.Logf("Warning: Invalid transfer address format, using range-only filter: %v", err)
		} else {
			senderAddr = &parsed
		}
	}

	b.Run("SQLite", func(b *testing.B) {
		db := loadSQLiteForBenchmark(b)
		defer db.Close()

		filter := &logsdb.TransferFilter{
			Range: &logsdb.Range{
				From: fromBlock,
				To:   toBlock,
			},
			CriteriaSet: []*logsdb.TransferCriteria{{
				Sender: senderAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterTransfers(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pebble", func(b *testing.B) {
		db := loadPebbleForBenchmark(b)
		defer db.Close()

		filter := &logsdb.TransferFilter{
			Range: &logsdb.Range{
				From: fromBlock,
				To:   toBlock,
			},
			CriteriaSet: []*logsdb.TransferCriteria{{
				Sender: senderAddr,
			}},
			Order: logsdb.ASC,
			Options: &logsdb.Options{
				Offset: 0,
				Limit:  1000,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := db.FilterTransfers(context.Background(), filter)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
