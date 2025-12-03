// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLite database path is now defined in shared_flags.go as SqliteDbPath

// Global variables for sync.Once discovery
var (
	discoveryOnce sync.Once
	discovered    *DiscoveryData
)

// DiscoveryData holds representative data discovered from the SQLite database
// This data is used for realistic API-style benchmarking
type DiscoveryData struct {
	// Event address categories
	HotAddresses    []string // Addresses with millions of logs
	MediumAddresses []string // Addresses with 10k-500k logs
	SparseAddresses []string // Addresses with <200 logs

	// Topic categories for each topic index
	HotTopics    []string // Topics with high frequency
	MediumTopics []string // Topics with medium frequency
	SparseTopics []string // Topics with low frequency

	// Complex multi-topic patterns for advanced benchmarks
	MultiTopicPatterns []EventPattern

	// Transfer address categories
	TransferHotAddresses    []string // TxOrigin/Sender/Recipient with many transfers
	TransferMediumAddresses []string // Addresses with moderate transfer activity
	TransferSparseAddresses []string // Addresses with few transfers
}

// EventPattern represents an event query pattern with multiple topics
// Empty strings indicate unused fields (NULL in database)
type EventPattern struct {
	Address string // Contract address, empty if not specified
	Topic0  string // Function signature topic
	Topic1  string // First indexed parameter
	Topic2  string // Second indexed parameter
	Topic3  string // Third indexed parameter
	Topic4  string // Fourth indexed parameter
}

// GetDiscoveryData returns cached discovery data, performing discovery once using sync.Once
// This is the main entry point for benchmarks to get realistic test data
// Now supports persistent caching to disk for faster subsequent runs
func GetDiscoveryData() *DiscoveryData {
	discoveryOnce.Do(func() {
		discovered = getDiscoveryDataWithCache()
	})
	return discovered
}

// getDiscoveryDataWithCache attempts to load from cache first, falls back to discovery
func getDiscoveryDataWithCache() *DiscoveryData {
	if *SqliteDbPath == "" {
		alwaysLogf("No database path provided, returning empty discovery data")
		return &DiscoveryData{}
	}

	// Validate discovery mode
	if *DiscoveryMode != "fast" && *DiscoveryMode != "full" {
		alwaysLogf("Invalid discovery mode '%s', using 'fast' mode. Valid options: fast, full", *DiscoveryMode)
		*DiscoveryMode = "fast"
	}

	alwaysLogf("Discovery mode: %s", *DiscoveryMode)

	// Generate cache file path based on mode
	var cacheFilePath string
	if *DiscoveryMode == "fast" {
		cacheFilePath = *SqliteDbPath + ".discovery-fast.json"
	} else {
		cacheFilePath = *SqliteDbPath + ".discovery-full.json"
	}

	// Also try legacy cache file (no mode suffix) for backward compatibility
	legacyCacheFilePath := *SqliteDbPath + ".discovery.json"

	// Try mode-specific cache first
	if data, loaded := tryLoadFromCache(cacheFilePath, *SqliteDbPath); loaded {
		alwaysLogf("Loaded %s mode cache from: %s", *DiscoveryMode, cacheFilePath)
		return data
	}

	// Try legacy cache as fallback (treat as full mode data)
	if data, loaded := tryLoadFromCache(legacyCacheFilePath, *SqliteDbPath); loaded {
		alwaysLogf("Loaded legacy cache (full mode) from: %s", legacyCacheFilePath)
		return data
	}

	// Fallback to discovery based on mode
	alwaysLogf("Cache miss - performing %s mode discovery...", *DiscoveryMode)
	data := performDiscovery()

	// Save to mode-specific cache for next time
	if err := saveToCache(data, cacheFilePath); err != nil {
		alwaysLogf("Warning: Failed to save discovery data to cache: %v", err)
	} else {
		alwaysLogf("Discovery data cached to: %s", cacheFilePath)
	}

	return data
}

// tryLoadFromCache attempts to load discovery data from cache file
// Returns (data, true) if successful, (empty, false) if cache miss
func tryLoadFromCache(cacheFilePath, dbPath string) (*DiscoveryData, bool) {
	// Check if cache file exists
	cacheInfo, err := os.Stat(cacheFilePath)
	if os.IsNotExist(err) {
		fmt.Printf("[%s] No cache file found at: %s\n", time.Now().Format("15:04:05.000"), cacheFilePath)
		return &DiscoveryData{}, false
	}
	if err != nil {
		fmt.Printf("[%s] Error checking cache file: %v\n", time.Now().Format("15:04:05.000"), err)
		return &DiscoveryData{}, false
	}

	// Check if database is newer than cache
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		fmt.Printf("[%s] Error checking database file: %v\n", time.Now().Format("15:04:05.000"), err)
		return &DiscoveryData{}, false
	}

	if dbInfo.ModTime().After(cacheInfo.ModTime()) {
		fmt.Printf("[%s] Database is newer than cache (DB: %v, Cache: %v) - cache invalidated\n",
			time.Now().Format("15:04:05.000"), dbInfo.ModTime().Format("2006-01-02 15:04:05"),
			cacheInfo.ModTime().Format("2006-01-02 15:04:05"))
		return &DiscoveryData{}, false
	}

	// Try to load and parse cache file
	fmt.Printf("[%s] Loading discovery data from cache: %s\n", time.Now().Format("15:04:05.000"), cacheFilePath)
	loadStart := time.Now()

	cacheData, err := os.ReadFile(cacheFilePath)
	if err != nil {
		fmt.Printf("[%s] Error reading cache file: %v\n", time.Now().Format("15:04:05.000"), err)
		return &DiscoveryData{}, false
	}

	var data DiscoveryData
	if err := json.Unmarshal(cacheData, &data); err != nil {
		fmt.Printf("[%s] Error parsing cache file: %v\n", time.Now().Format("15:04:05.000"), err)
		return &DiscoveryData{}, false
	}

	loadDuration := time.Since(loadStart)

	// Log cache hit statistics
	totalAddresses := len(data.HotAddresses) + len(data.MediumAddresses) + len(data.SparseAddresses) +
		len(data.TransferHotAddresses) + len(data.TransferMediumAddresses) + len(data.TransferSparseAddresses)
	totalTopics := len(data.HotTopics) + len(data.MediumTopics) + len(data.SparseTopics)

	alwaysLogf("✅ Cache hit! Loaded in %v", loadDuration)
	alwaysLogf("  - Addresses: %d, Topics: %d, Patterns: %d",
		totalAddresses, totalTopics, len(data.MultiTopicPatterns))

	return &data, true
}

// saveToCache saves discovery data to cache file in JSON format
func saveToCache(data *DiscoveryData, cacheFilePath string) error {
	saveStart := time.Now()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal discovery data: %v", err)
	}

	if err := os.WriteFile(cacheFilePath, jsonData, 0o644); err != nil { //nolint
		return fmt.Errorf("failed to write cache file: %v", err)
	}

	saveDuration := time.Since(saveStart)
	fmt.Printf("[%s] Discovery data saved to cache in %v (size: %.1f KB)\n",
		time.Now().Format("15:04:05.000"), saveDuration, float64(len(jsonData))/1024)

	return nil
}

// performDiscovery executes the actual database discovery process
// This is called exactly once by sync.Once
func performDiscovery() *DiscoveryData {
	startTime := time.Now()
	fmt.Printf("[%s] Starting benchmark data discovery process...\n", startTime.Format("15:04:05.000"))

	// Initialize fixed random seed for reproducible results
	rand.Seed(DISCOVERY_SEED) // nolint
	fmt.Printf("[%s] Initialized random seed (%d) for reproducible results\n", time.Now().Format("15:04:05.000"), DISCOVERY_SEED)

	// Validate database path and connectivity
	logf("Validating database path: %s", *SqliteDbPath)
	if err := validateDatabase(*SqliteDbPath); err != nil {
		fmt.Printf("[%s] Discovery failed: %v\n", time.Now().Format("15:04:05.000"), err)
		return &DiscoveryData{} // Return empty data rather than nil
	}
	fmt.Printf("[%s] Database validation successful\n", time.Now().Format("15:04:05.000"))

	// Open SQLite database
	fmt.Printf("[%s] Opening SQLite database: %s\n", time.Now().Format("15:04:05.000"), *SqliteDbPath)
	db, err := sql.Open("sqlite3", *SqliteDbPath)
	if err != nil {
		fmt.Printf("[%s] Failed to open database: %v\n", time.Now().Format("15:04:05.000"), err)
		return &DiscoveryData{}
	}
	defer db.Close()
	fmt.Printf("[%s] Database connection established\n", time.Now().Format("15:04:05.000"))

	// Initialize discovery data structure
	data := &DiscoveryData{}

	// Execute event address discovery using optimized sampling
	fmt.Printf("[%s] === Starting Event Address Discovery ===\n", time.Now().Format("15:04:05.000"))
	phaseStart := time.Now()
	data.HotAddresses, data.MediumAddresses, data.SparseAddresses, _ = queryAddressSample(db)
	fmt.Printf("[%s] Event address discovery completed in %v\n", time.Now().Format("15:04:05.000"), time.Since(phaseStart))
	fmt.Printf("[%s]   - Hot addresses: %d, Medium: %d, Sparse: %d\n", time.Now().Format("15:04:05.000"),
		len(data.HotAddresses), len(data.MediumAddresses), len(data.SparseAddresses))

	// Execute topic discovery using optimized sampling
	fmt.Printf("[%s] === Starting Topic Discovery ===\n", time.Now().Format("15:04:05.000"))
	phaseStart = time.Now()
	data.HotTopics, data.MediumTopics, data.SparseTopics, _ = queryTopicSample(db)
	fmt.Printf("[%s] Topic discovery completed in %v\n", time.Now().Format("15:04:05.000"), time.Since(phaseStart))
	fmt.Printf("[%s]   - Hot topics: %d, Medium: %d, Sparse: %d\n", time.Now().Format("15:04:05.000"),
		len(data.HotTopics), len(data.MediumTopics), len(data.SparseTopics))

	// Execute multi-topic pattern discovery
	fmt.Printf("[%s] === Starting Multi-Topic Pattern Discovery ===\n", time.Now().Format("15:04:05.000"))
	phaseStart = time.Now()
	data.MultiTopicPatterns, _ = queryMultiTopicPatterns(db)
	fmt.Printf("[%s] Multi-topic pattern discovery completed in %v\n", time.Now().Format("15:04:05.000"), time.Since(phaseStart))
	fmt.Printf("[%s]   - Multi-topic patterns found: %d\n", time.Now().Format("15:04:05.000"), len(data.MultiTopicPatterns))

	// Execute transfer address discovery using optimized sampling
	fmt.Printf("[%s] === Starting Transfer Address Discovery ===\n", time.Now().Format("15:04:05.000"))
	phaseStart = time.Now()
	data.TransferHotAddresses, data.TransferMediumAddresses, data.TransferSparseAddresses, _ = queryTransferSample(db)
	fmt.Printf("[%s] Transfer address discovery completed in %v\n", time.Now().Format("15:04:05.000"), time.Since(phaseStart))
	fmt.Printf("[%s]   - Hot addresses: %d, Medium: %d, Sparse: %d\n", time.Now().Format("15:04:05.000"),
		len(data.TransferHotAddresses), len(data.TransferMediumAddresses), len(data.TransferSparseAddresses))

	// Final summary
	totalDuration := time.Since(startTime)
	totalAddresses := len(data.HotAddresses) + len(data.MediumAddresses) + len(data.SparseAddresses) +
		len(data.TransferHotAddresses) + len(data.TransferMediumAddresses) + len(data.TransferSparseAddresses)
	totalTopics := len(data.HotTopics) + len(data.MediumTopics) + len(data.SparseTopics)

	fmt.Printf("[%s] === Discovery Complete ===\n", time.Now().Format("15:04:05.000"))
	fmt.Printf("[%s] Total addresses discovered: %d\n", time.Now().Format("15:04:05.000"), totalAddresses)
	fmt.Printf("[%s] Total topics discovered: %d\n", time.Now().Format("15:04:05.000"), totalTopics)
	fmt.Printf("[%s] Total multi-topic patterns: %d\n", time.Now().Format("15:04:05.000"), len(data.MultiTopicPatterns))
	fmt.Printf("[%s] Total discovery time: %v\n", time.Now().Format("15:04:05.000"), totalDuration)

	return data
}

// validateDatabase checks that the database path is valid and accessible
func validateDatabase(path string) error {
	if path == "" {
		return fmt.Errorf("database path is required (use -discoveryDbPath flag)")
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("database file does not exist: %s", path)
	}

	// Test SQLite connectivity
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return fmt.Errorf("failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Verify we can query the database
	if err := db.Ping(); err != nil {
		return fmt.Errorf("database is not accessible: %v", err)
	}

	return nil
}

// AddressSample represents an address with its sample count
type AddressSample struct {
	Address string
	Count   int64
}

// queryAddressSample performs sampling-based address discovery for all tiers
// Uses rowid % 97 = 0 for ~1% statistical sample to avoid full table scans
func queryAddressSample(db *sql.DB) (hot, medium, sparse []string, err error) {
	var query string
	var queryDescription string

	if *DiscoveryMode == "fast" {
		// Fast mode: Use index-friendly range sampling
		queryDescription = "index-friendly address discovery (multiple ranges)"
		logf("  -> Executing %s...", queryDescription)
		
		query = `
			SELECT LOWER('0x' || HEX(r.data)) as address, COUNT(*) as cnt 
			FROM event e
			JOIN ref r ON e.address = r.id
			WHERE (e.seq BETWEEN 1000000 AND 1010000)
			   OR (e.seq BETWEEN 2000000 AND 2010000) 
			   OR (e.seq BETWEEN 3000000 AND 3010000)
			   OR (e.seq BETWEEN 5000000 AND 5010000)
			   OR (e.seq BETWEEN 8000000 AND 8010000)
			   OR (e.seq BETWEEN 13000000 AND 13010000)
			   OR (e.seq BETWEEN 21000000 AND 21010000)
			   OR (e.seq BETWEEN 34000000 AND 34010000)
			GROUP BY e.address 
			ORDER BY cnt DESC 
			LIMIT 1000`
	} else {
		// Full mode: Use modulo sampling (original behavior)
		queryDescription = "sampling-based address discovery (rowid % 97 = 0)"
		logf("  -> Executing %s...", queryDescription)
		
		query = `
			SELECT LOWER('0x' || HEX(r.data)) as address, COUNT(*) as cnt 
			FROM event e
			JOIN ref r ON e.address = r.id
			WHERE e.rowid % 97 = 0 
			GROUP BY e.address 
			ORDER BY cnt DESC 
			LIMIT 5000`
	}

	queryStart := time.Now()
	rows, err := db.Query(query)
	if err != nil {
		alwaysLogf("  -> %s failed: %v", queryDescription, err)
		return nil, nil, nil, err
	}
	defer rows.Close()

	var samples []AddressSample
	var maxCount, minCount int64 = 0, -1

	for rows.Next() {
		var sample AddressSample
		if err := rows.Scan(&sample.Address, &sample.Count); err != nil {
			continue // Skip problematic rows
		}
		samples = append(samples, sample)

		// Track statistics
		if len(samples) == 1 {
			maxCount = sample.Count
		}
		minCount = sample.Count
	}

	queryDuration := time.Since(queryStart)
	logf("  -> %s completed in %v", queryDescription, queryDuration)
	logf("    Sample size: %d addresses (range: %d - %d sample events each)", len(samples), minCount, maxCount)

	// Classify addresses into tiers based on sample counts
	// Hot: sample_count >= 50 (extrapolates to ~5000+ actual events)
	// Medium: 5 <= sample_count < 50 (500-5000 actual events)
	// Sparse: 1 <= sample_count < 5 (100-500 actual events)
	for _, sample := range samples {
		if sample.Count >= 50 {
			hot = append(hot, sample.Address)
		} else if sample.Count >= 5 {
			medium = append(medium, sample.Address)
		} else if sample.Count >= 1 {
			sparse = append(sparse, sample.Address)
		}
	}

	totalAddresses := len(hot) + len(medium) + len(sparse)

	// Fast mode fallback: if insufficient addresses found, try reduced modulo sampling
	if *DiscoveryMode == "fast" && totalAddresses < 10 {
		alwaysLogf("  -> Fast mode found only %d addresses, falling back to reduced modulo sampling...", totalAddresses)
		
		fallbackQuery := `
			SELECT LOWER('0x' || HEX(r.data)) as address, COUNT(*) as cnt 
			FROM event e
			JOIN ref r ON e.address = r.id
			WHERE e.rowid % 997 = 0 
			GROUP BY e.address 
			ORDER BY cnt DESC 
			LIMIT 5000`
		
		fallbackStart := time.Now()
		fallbackRows, err := db.Query(fallbackQuery)
		if err != nil {
			alwaysLogf("  -> Fallback query failed: %v", err)
			// Continue with what we found from fast mode
		} else {
			defer fallbackRows.Close()

			// Re-process with fallback results
			var fallbackSamples []AddressSample

			for fallbackRows.Next() {
				var sample AddressSample
				if err := fallbackRows.Scan(&sample.Address, &sample.Count); err != nil {
					continue
				}
				fallbackSamples = append(fallbackSamples, sample)
			}

			fallbackDuration := time.Since(fallbackStart)
			alwaysLogf("  -> Fallback sampling completed in %v", fallbackDuration)
			alwaysLogf("    Fallback sample size: %d addresses", len(fallbackSamples))

			// Re-classify with fallback data if we got better results
			if len(fallbackSamples) > totalAddresses {
				hot, medium, sparse = []string{}, []string{}, []string{}
				for _, sample := range fallbackSamples {
					if sample.Count >= 50 {
						hot = append(hot, sample.Address)
					} else if sample.Count >= 5 {
						medium = append(medium, sample.Address)
					} else if sample.Count >= 1 {
						sparse = append(sparse, sample.Address)
					}
				}
				alwaysLogf("    Fallback improved results: Hot=%d, Medium=%d, Sparse=%d", len(hot), len(medium), len(sparse))
			}
		}
	}

	// Randomize and limit selections
	hot = randomizeSlice(hot, 10)       // Top hot addresses
	medium = randomizeSlice(medium, 30) // Medium activity addresses
	sparse = randomizeSlice(sparse, 50) // Lower activity addresses

	alwaysLogf("    Final tier classification: Hot=%d, Medium=%d, Sparse=%d", len(hot), len(medium), len(sparse))
	return hot, medium, sparse, nil
}

// TopicSample represents a topic with its sample count and position
type TopicSample struct {
	Topic    string
	Position int // 0-4 for topic0-topic4
	Count    int64
}

// queryTopicSample performs sampling-based topic discovery for all positions and tiers
// Uses single query with rowid % 97 = 0 to avoid multiple full table scans
func queryTopicSample(db *sql.DB) (hot, medium, sparse []string, err error) {
	fmt.Printf("[%s]   -> Executing sampling-based topic discovery (rowid %% 97 = 0)...\n", time.Now().Format("15:04:05.000"))
	queryStart := time.Now()

	// Single query to sample ~1% of events for all topic positions
	// Use subqueries to convert topic IDs to hex strings
	query := `
		SELECT 
			CASE WHEN topic0 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic0))) END as topic0,
			CASE WHEN topic1 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic1))) END as topic1,
			CASE WHEN topic2 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic2))) END as topic2,
			CASE WHEN topic3 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic3))) END as topic3,
			CASE WHEN topic4 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic4))) END as topic4
		FROM event 
		WHERE rowid % 97 = 0 
		  AND (topic0 IS NOT NULL OR topic1 IS NOT NULL OR topic2 IS NOT NULL 
		       OR topic3 IS NOT NULL OR topic4 IS NOT NULL)
		LIMIT 100000`

	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("[%s]   -> Topic sampling query failed: %v\n", time.Now().Format("15:04:05.000"), err)
		return nil, nil, nil, err
	}
	defer rows.Close()

	// Count topic frequencies from sample
	topicCounts := make(map[string]int64)
	sampleCount := 0

	for rows.Next() {
		var topics [5]*string // topic0-topic4
		if err := rows.Scan(&topics[0], &topics[1], &topics[2], &topics[3], &topics[4]); err != nil {
			continue
		}

		// Count each non-null topic
		for _, topic := range topics {
			if topic != nil && *topic != "" {
				topicCounts[*topic]++
			}
		}
		sampleCount++
	}

	queryDuration := time.Since(queryStart)
	fmt.Printf("[%s]   -> Topic sampling completed in %v\n", time.Now().Format("15:04:05.000"), queryDuration)
	fmt.Printf("[%s]     Processed %d sample events, found %d unique topics\n", time.Now().Format("15:04:05.000"), sampleCount, len(topicCounts))

	// Convert to sorted slice
	var samples []TopicSample
	for topic, count := range topicCounts {
		samples = append(samples, TopicSample{Topic: topic, Count: count})
	}

	// Sort by frequency
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Count > samples[j].Count
	})

	// Classify topics into tiers based on sample counts
	// Hot: sample_count >= 20 (high frequency across sample)
	// Medium: 5 <= sample_count < 20 (medium frequency)
	// Sparse: 1 <= sample_count < 5 (low frequency)
	for _, sample := range samples {
		if sample.Count >= 20 {
			hot = append(hot, sample.Topic)
		} else if sample.Count >= 5 {
			medium = append(medium, sample.Topic)
		} else if sample.Count >= 1 {
			sparse = append(sparse, sample.Topic)
		}
	}

	// Randomize and limit selections
	hot = randomizeSlice(hot, 25)       // Top frequent topics
	medium = randomizeSlice(medium, 40) // Medium frequency topics
	sparse = randomizeSlice(sparse, 60) // Lower frequency topics

	fmt.Printf("[%s]     Tier classification: Hot=%d, Medium=%d, Sparse=%d\n", time.Now().Format("15:04:05.000"), len(hot), len(medium), len(sparse))
	return hot, medium, sparse, nil
}

// queryMultiTopicPatterns performs sampling-based multi-topic pattern discovery
// Uses single sample from rowid % 97 = 0 to generate various topic combination patterns
func queryMultiTopicPatterns(db *sql.DB) ([]EventPattern, error) {
	fmt.Printf("[%s]   -> Executing sampling-based multi-topic pattern discovery...\\n", time.Now().Format("15:04:05.000"))
	queryStart := time.Now()

	// Single query to sample events with multiple topics - much more efficient than DISTINCT queries
	// Join with ref table to convert all IDs to hex strings
	query := `
		SELECT 
			LOWER('0x' || HEX(ra.data)) as address,
			CASE WHEN topic0 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic0))) END as topic0,
			CASE WHEN topic1 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic1))) END as topic1,
			CASE WHEN topic2 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic2))) END as topic2,
			CASE WHEN topic3 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic3))) END as topic3,
			CASE WHEN topic4 IS NOT NULL THEN LOWER('0x' || HEX((SELECT data FROM ref WHERE id = topic4))) END as topic4
		FROM event e
		JOIN ref ra ON e.address = ra.id
		WHERE e.rowid % 97 = 0 
		  AND (topic0 IS NOT NULL OR topic1 IS NOT NULL OR topic2 IS NOT NULL 
		       OR topic3 IS NOT NULL OR topic4 IS NOT NULL)
		LIMIT 10000`

	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("[%s]   -> Multi-topic pattern sampling failed: %v\\n", time.Now().Format("15:04:05.000"), err)
		return nil, err
	}
	defer rows.Close()

	var allSamples []EventPattern
	sampleCount := 0

	for rows.Next() {
		var sample EventPattern
		var topic0, topic1, topic2, topic3, topic4 sql.NullString
		if err := rows.Scan(&sample.Address, &topic0, &topic1, &topic2, &topic3, &topic4); err != nil {
			continue
		}

		// Convert nullable strings to EventPattern format
		if topic0.Valid {
			sample.Topic0 = topic0.String
		}
		if topic1.Valid {
			sample.Topic1 = topic1.String
		}
		if topic2.Valid {
			sample.Topic2 = topic2.String
		}
		if topic3.Valid {
			sample.Topic3 = topic3.String
		}
		if topic4.Valid {
			sample.Topic4 = topic4.String
		}

		allSamples = append(allSamples, sample)
		sampleCount++
	}

	queryDuration := time.Since(queryStart)
	fmt.Printf("[%s]   -> Pattern sampling completed in %v\\n", time.Now().Format("15:04:05.000"), queryDuration)
	fmt.Printf("[%s]     Sampled %d events with topics\\n", time.Now().Format("15:04:05.000"), sampleCount)

	// Extract different pattern types from the sample
	var patterns []EventPattern

	// 2-topic patterns (primary focus)
	twoTopicCount := 0
	for _, sample := range allSamples {
		topicCount := countNonEmptyTopics(sample)
		if topicCount >= 2 && len(patterns) < 50 {
			patterns = append(patterns, sample)
			twoTopicCount++
		}
	}

	// 3-topic patterns (secondary focus)
	threeTopicCount := 0
	for _, sample := range allSamples {
		topicCount := countNonEmptyTopics(sample)
		if topicCount >= 3 && threeTopicCount < 20 {
			patterns = append(patterns, sample)
			threeTopicCount++
		}
	}

	// 4+ topic patterns (if they exist)
	fourTopicCount := 0
	for _, sample := range allSamples {
		topicCount := countNonEmptyTopics(sample)
		if topicCount >= 4 && fourTopicCount < 10 {
			patterns = append(patterns, sample)
			fourTopicCount++
		}
	}

	fmt.Printf("[%s]     Pattern extraction: 2-topic=%d, 3-topic=%d, 4+topic=%d\\n",
		time.Now().Format("15:04:05.000"), twoTopicCount, threeTopicCount, fourTopicCount)

	// Final randomization and limit
	randomized := randomizePatterns(patterns, 80)
	fmt.Printf("[%s]     Final selection: %d multi-topic patterns chosen\\n", time.Now().Format("15:04:05.000"), len(randomized))
	return randomized, nil
}

// countNonEmptyTopics counts how many topics are non-empty in a pattern
func countNonEmptyTopics(pattern EventPattern) int {
	count := 0
	if pattern.Topic0 != "" {
		count++
	}
	if pattern.Topic1 != "" {
		count++
	}
	if pattern.Topic2 != "" {
		count++
	}
	if pattern.Topic3 != "" {
		count++
	}
	if pattern.Topic4 != "" {
		count++
	}
	return count
}

// randomizePatterns randomly selects patterns from the input slice
func randomizePatterns(patterns []EventPattern, maxCount int) []EventPattern {
	if len(patterns) <= maxCount {
		return patterns
	}

	indices := rand.Perm(len(patterns))
	result := make([]EventPattern, maxCount)
	for i := 0; i < maxCount; i++ {
		result[i] = patterns[indices[i]]
	}
	return result
}

// TransferSample represents a transfer address with its sample count
type TransferSample struct {
	Address string
	Count   int64
	Type    string // "txOrigin", "sender", "recipient"
}

// queryTransferSample performs sampling-based transfer address discovery for all tiers
// Uses rowid % 97 = 0 for ~1% statistical sample to avoid full table scans
func queryTransferSample(db *sql.DB) (hot, medium, sparse []string, err error) {
	fmt.Printf("[%s]   -> Executing sampling-based transfer address discovery (rowid %% 97 = 0)...\\n", time.Now().Format("15:04:05.000"))
	queryStart := time.Now()

	// Single query to sample ~1% of transfers and collect all address types
	// Join with ref table to convert address IDs to hex strings
	query := `
		SELECT 
			LOWER('0x' || HEX(r1.data)) as txOrigin,
			LOWER('0x' || HEX(r2.data)) as sender,
			LOWER('0x' || HEX(r3.data)) as recipient
		FROM transfer t
		JOIN ref r1 ON t.txOrigin = r1.id
		JOIN ref r2 ON t.sender = r2.id
		JOIN ref r3 ON t.recipient = r3.id
		WHERE t.rowid % 97 = 0 
		LIMIT 50000`

	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("[%s]   -> Transfer sampling query failed: %v\\n", time.Now().Format("15:04:05.000"), err)
		return nil, nil, nil, err
	}
	defer rows.Close()

	// Count address frequencies across all roles
	addressCounts := make(map[string]int64)
	sampleCount := 0

	for rows.Next() {
		var txOrigin, sender, recipient string
		if err := rows.Scan(&txOrigin, &sender, &recipient); err != nil {
			continue
		}

		// Count each address in its role
		if txOrigin != "" {
			addressCounts[txOrigin]++
		}
		if sender != "" {
			addressCounts[sender]++
		}
		if recipient != "" {
			addressCounts[recipient]++
		}
		sampleCount++
	}

	queryDuration := time.Since(queryStart)
	fmt.Printf("[%s]   -> Transfer sampling completed in %v\\n", time.Now().Format("15:04:05.000"), queryDuration)
	fmt.Printf("[%s]     Processed %d transfer samples, found %d unique addresses\\n", time.Now().Format("15:04:05.000"), sampleCount, len(addressCounts))

	// Convert to sorted slice
	var samples []TransferSample
	for address, count := range addressCounts {
		samples = append(samples, TransferSample{Address: address, Count: count})
	}

	// Sort by frequency
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Count > samples[j].Count
	})

	// Classify addresses into tiers based on sample counts
	// Hot: sample_count >= 10 (high transfer activity across sample)
	// Medium: 3 <= sample_count < 10 (medium transfer activity)
	// Sparse: 1 <= sample_count < 3 (low transfer activity)
	for _, sample := range samples {
		if sample.Count >= 10 {
			hot = append(hot, sample.Address)
		} else if sample.Count >= 3 {
			medium = append(medium, sample.Address)
		} else if sample.Count >= 1 {
			sparse = append(sparse, sample.Address)
		}
	}

	// Randomize and limit selections
	hot = randomizeSlice(hot, 25)       // Top transfer addresses
	medium = randomizeSlice(medium, 40) // Medium transfer addresses
	sparse = randomizeSlice(sparse, 60) // Lower transfer addresses

	fmt.Printf("[%s]     Tier classification: Hot=%d, Medium=%d, Sparse=%d\\n", time.Now().Format("15:04:05.000"), len(hot), len(medium), len(sparse))
	return hot, medium, sparse, nil
}

// randomizeSlice randomly selects items from a string slice
// This provides variation in benchmark data while maintaining determinism via fixed seed
func randomizeSlice(slice []string, maxCount int) []string {
	if len(slice) <= maxCount {
		return slice
	}

	indices := rand.Perm(len(slice))
	result := make([]string, maxCount)
	for i := 0; i < maxCount; i++ {
		result[i] = slice[indices[i]]
	}
	return result
}
