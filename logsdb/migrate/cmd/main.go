package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vechain/thor/v2/logsdb/migrate"
)

func main() {
	// mainnet db
	const (
		TOTAL_EVENTS    = 703698865
		TOTAL_TRANSFERS = 18143572
		sqliteDBPath    = "/Volumes/vechain/mainnet/data/instance-39627e6be7ec1b4a-v4/logs-v2.db"
		pebbleDBPath    = "/Volumes/vechain/mainnet/data/instance-39627e6be7ec1b4a-v4/pebbledb.db"
	)

	//// testnet db
	//const (
	//	TOTAL_EVENTS    = 247065997
	//	TOTAL_TRANSFERS = 7452777
	//
	//	sqliteDBPath = "/Volumes/vechain/testnet/data/instance-7f466536b20bb127-v4/logs-v2.db"
	//	pebbleDBPath = "/Volumes/vechain/testnet/data/instance-7f466536b20bb127-v4/pebbledb.db"
	//)

	// Command line flags
	var (
		withTotals = flag.Bool("totals", false, "Use pre-known totals to skip counting phase")
		help       = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printUsage()
		return
	}

	var stats *migrate.MigrationStats
	var err error

	fmt.Printf("🚀 Starting PebbleDB migration with streaming query engine...\n")
	if *withTotals {
		fmt.Println("Using pre-known totals (skipping 19+ minute counting phase)...")
		stats, err = migrate.MigrateSQLiteToPebbleUltraFastWithTotals(
			sqliteDBPath,
			pebbleDBPath,
			TOTAL_EVENTS,
			TOTAL_TRANSFERS,
		)
	} else {
		fmt.Println("Starting ultra-fast PebbleDB migration...")
		stats, err = migrate.MigrateSQLiteToPebbleUltraFast(
			sqliteDBPath,
			pebbleDBPath,
		)
	}

	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Print results
	fmt.Printf("\n🎉 PebbleDB Migration completed successfully!\n")
	fmt.Printf("📊 Performance: %.0f events/sec, %.0f transfers/sec\n", stats.EventsPerSecond, stats.TransfersPerSecond)
	fmt.Printf("⏱️  Total time: %v\n", stats.Duration)
	fmt.Printf("💾 Size reduction: %.1f GB → %.1f GB\n", float64(stats.SourceSize)/1e9, float64(stats.TargetSize)/1e9)
}

func printUsage() {
	fmt.Printf("Usage: %s [options]\n", os.Args[0])
	fmt.Printf("\nMigration tool for SQLite LogDB to PebbleDB/PebbleDB\n\n")
	fmt.Printf("Options:\n")
	fmt.Printf("  -totals    Use pre-known totals (skips ~19min counting phase)\n")
	fmt.Printf("  -help      Show this help message\n")
	fmt.Printf("\nExamples:\n")
	fmt.Printf("  %s                    # Migrate to PebbleDB (original)\n", os.Args[0])
	fmt.Printf("  %s -v3               # Migrate to PebbleDB (streaming)\n", os.Args[0])
	fmt.Printf("  %s -v3 -totals       # Migrate to PebbleDB with known totals (fastest)\n", os.Args[0])
	fmt.Printf("  %s -totals           # Migrate to PebbleDB with known totals\n", os.Args[0])
}
