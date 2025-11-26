package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vechain/thor/v2/logdb/test"
)

func main() {
	// // mainnet db
	//const (
	//	TOTAL_EVENTS    = 703698865
	//	TOTAL_TRANSFERS = 18143572
	//	sqlitePath      = "/Volumes/vechain/mainnet/data/instance-39627e6be7ec1b4a-v4-full/logs-v2.db"
	//	pebblePath      = "/Volumes/vechain/mainnet/data/instance-39627e6be7ec1b4a-v4-full/pebble.db"
	//	pebbleV3Path    = "/Volumes/vechain/mainnet/data/instance-39627e6be7ec1b4a-v4-full/pebblev3.db"
	//)

	// testnet db
	const (
		TOTAL_EVENTS    = 247036872
		TOTAL_TRANSFERS = 7450530

		sqlitePath   = "/Volumes/vechain/testnet/data/instance-7f466536b20bb127-v4/logs-v2.db"
		pebblePath   = "/Volumes/vechain/testnet/data/instance-7f466536b20bb127-v4/pebble.db"
		pebbleV3Path = "/Volumes/vechain/testnet/data/instance-7f466536b20bb127-v4/pebblev3.db"
	)

	// Command line flags
	var (
		usePebbleV3 = flag.Bool("v3", false, "Migrate to PebbleV3 instead of PebbleDB")
		withTotals  = flag.Bool("totals", false, "Use pre-known totals to skip counting phase")
		help        = flag.Bool("help", false, "Show help message")
	)
	flag.Parse()

	if *help {
		printUsage()
		return
	}

	var stats *test.MigrationStats
	var err error

	if *usePebbleV3 {
		fmt.Printf("🚀 Starting PebbleV3 migration with streaming query engine...\n")
		if *withTotals {
			fmt.Println("Using pre-known totals (skipping 19+ minute counting phase)...")
			stats, err = test.MigrateSQLiteToPebbleUltraFastWithTotals(
				sqlitePath,
				pebbleV3Path,
				TOTAL_EVENTS,
				TOTAL_TRANSFERS,
			)
		} else {
			fmt.Println("Starting ultra-fast PebbleV3 migration...")
			stats, err = test.MigrateSQLiteToPebbleUltraFast(
				sqlitePath,
				pebbleV3Path,
			)
		}
	} else {
		fmt.Printf("🗂️  Starting PebbleDB migration (original implementation)...\n")
		if *withTotals {
			fmt.Println("Using pre-known totals (skipping 19+ minute counting phase)...")
			stats, err = test.MigrateSQLiteToPebbleUltraFastWithTotals(
				sqlitePath,
				pebblePath,
				TOTAL_EVENTS,
				TOTAL_TRANSFERS,
			)
		} else {
			fmt.Println("Starting ultra-fast migration...")
			stats, err = test.MigrateSQLiteToPebbleUltraFast(
				sqlitePath,
				pebblePath,
			)
		}
	}

	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Print results with appropriate database type
	dbType := "PebbleDB"
	if *usePebbleV3 {
		dbType = "PebbleV3"
	}

	fmt.Printf("\n🎉 %s Migration completed successfully!\n", dbType)
	fmt.Printf("📊 Performance: %.0f events/sec, %.0f transfers/sec\n", stats.EventsPerSecond, stats.TransfersPerSecond)
	fmt.Printf("⏱️  Total time: %v\n", stats.Duration)
	fmt.Printf("💾 Size reduction: %.1f GB → %.1f GB\n", float64(stats.SourceSize)/1e9, float64(stats.TargetSize)/1e9)

	if *usePebbleV3 {
		fmt.Printf("\n✨ PebbleV3 Features:\n")
		fmt.Printf("  • Streaming query execution with bounded memory\n")
		fmt.Printf("  • Optimized AND/OR semantics with leapfrog intersection\n")
		fmt.Printf("  • Precise truncate operations with proper index handling\n")
		fmt.Printf("  • Better performance for complex multi-criteria queries\n")
	}
}

func printUsage() {
	fmt.Printf("Usage: %s [options]\n", os.Args[0])
	fmt.Printf("\nMigration tool for SQLite LogDB to PebbleDB/PebbleV3\n\n")
	fmt.Printf("Options:\n")
	fmt.Printf("  -v3        Migrate to PebbleV3 (streaming query engine)\n")
	fmt.Printf("  -totals    Use pre-known totals (skips ~19min counting phase)\n")
	fmt.Printf("  -help      Show this help message\n")
	fmt.Printf("\nExamples:\n")
	fmt.Printf("  %s                    # Migrate to PebbleDB (original)\n", os.Args[0])
	fmt.Printf("  %s -v3               # Migrate to PebbleV3 (streaming)\n", os.Args[0])
	fmt.Printf("  %s -v3 -totals       # Migrate to PebbleV3 with known totals (fastest)\n", os.Args[0])
	fmt.Printf("  %s -totals           # Migrate to PebbleDB with known totals\n", os.Args[0])
}
