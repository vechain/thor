// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package muxdb implements the storage layer for block-chain.
// It manages instance of merkle-patricia-trie, and general purpose named kv-store.
package muxdb

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/vechain/thor/v2/metrics"
)

var (
	metricCacheHitMiss = metrics.LazyLoadGaugeVec("cache_hit_miss_count", []string{"type", "event"})
	metricCompaction   = metrics.LazyLoadGaugeVec("compaction_stats_gauge", []string{"level", "type"})
)

// CompactionValues holds the values for a specific level.
type CompactionValues struct {
	Level   string
	Tables  int64
	SizeMB  int64
	TimeSec int64
	ReadMB  int64
	WriteMB int64
}

// Collects the compaction values from the stats table.
// The format of the stats table is:
/*
Compactions
 Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
-------+------------+---------------+---------------+---------------+---------------
   0   |          2 |     224.46577 |       3.25844 |       0.00000 |    1908.26756
   1   |         29 |     110.98547 |       6.76062 |    2070.73768 |    2054.52797
   2   |        295 |    1109.32673 |       3.16157 |     883.22560 |     799.85596
   3   |       2777 |   10206.97173 |       0.33533 |     103.17983 |      91.55081
   4   |       4100 |   15773.54834 |       6.75241 |    2032.57337 |    1851.48528
-------+------------+---------------+---------------+---------------+---------------
 Total |       7203 |   27425.29804 |      20.26837 |    5089.71648 |    6705.68758

*/
func collectCompactionValues(stats string) {
	// Create a new tabwriter
	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 1, ' ', tabwriter.Debug)

	// Print the stats string using the tabwriter
	fmt.Fprintln(w, stats)
	w.Flush()

	// Extract and log the value from the specified level
	formattedStats := sb.String()
	logger.Debug(formattedStats)
	values, err := extractCompactionValues(formattedStats)
	if err != nil {
		logger.Error("Failed to extract values for stats %s: %v", stats, err)
	} else {
		for _, value := range values {
			metricCompaction().SetWithLabel(value.Tables, map[string]string{"level": value.Level, "type": "tables"})
			metricCompaction().SetWithLabel(value.SizeMB, map[string]string{"level": value.Level, "type": "size-mb"})
			metricCompaction().SetWithLabel(value.TimeSec, map[string]string{"level": value.Level, "type": "time-sec"})
			metricCompaction().SetWithLabel(value.ReadMB, map[string]string{"level": value.Level, "type": "read-mb"})
			metricCompaction().SetWithLabel(value.WriteMB, map[string]string{"level": value.Level, "type": "write-mb"})
		}
	}
}

func parseAndRoundFloatToInt64(str string) (int64, error) {
	// Parse the string to a float64
	floatValue, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, err
	}

	// Round the float64 value
	roundedValue := math.Round(floatValue)

	// Convert the rounded float64 to int64
	intValue := int64(roundedValue)

	return intValue, nil
}

func extractCompactionValues(stats string) ([]CompactionValues, error) {
	lines := strings.Split(stats, "\n")
	var values []CompactionValues

	for _, line := range lines[2 : len(lines)-3] {
		columns := strings.Fields(line)
		if len(columns) >= 6 {
			value, err := parseCompactionColumns(columns)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("no valid compaction values found in stats %s", stats)
	}

	return values, nil
}

func parseCompactionColumns(columns []string) (CompactionValues, error) {
	tables, err := strconv.ParseInt(columns[2], 10, 64)
	if err != nil {
		return CompactionValues{}, fmt.Errorf("error when parsing tables: %v", err)
	}
	sizeMb, err := parseAndRoundFloatToInt64(columns[4])
	if err != nil {
		return CompactionValues{}, fmt.Errorf("error when parsing sizeMb: %v", err)
	}
	timeSec, err := parseAndRoundFloatToInt64(columns[6])
	if err != nil {
		return CompactionValues{}, fmt.Errorf("error when parsing timeSec: %v", err)
	}
	readMb, err := parseAndRoundFloatToInt64(columns[8])
	if err != nil {
		return CompactionValues{}, fmt.Errorf("error when parsing readMb: %v", err)
	}
	writeMb, err := parseAndRoundFloatToInt64(columns[10])
	if err != nil {
		return CompactionValues{}, fmt.Errorf("error when parsing writeMb: %v", err)
	}
	return CompactionValues{
		Level:   columns[0],
		Tables:  tables,
		SizeMB:  sizeMb,
		TimeSec: timeSec,
		ReadMB:  readMb,
		WriteMB: writeMb,
	}, nil
}
