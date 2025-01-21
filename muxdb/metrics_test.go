// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package muxdb implements the storage layer for block-chain.
// It manages instance of merkle-patricia-trie, and general purpose named kv-store.
package muxdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsCollectCompactionValues(t *testing.T) {
	require.NotPanics(t, func() { collectCompactionValues("") })
	require.NotPanics(t, func() { collectCompactionValues("wrong stats") })

	stats := ` Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
-------+------------+---------------+---------------+---------------+---------------
   0   |          0 |       0.00000 |       0.16840 |       0.00000 |      61.67909
   1   |         27 |      96.34199 |       1.03280 |     139.39040 |     138.68919
   2   |        271 |     989.34527 |       0.15046 |      45.49008 |      39.92714
   3   |       2732 |   10002.10112 |       1.11660 |     128.58780 |     119.32566
   4   |       3544 |   13591.24199 |       3.38804 |    2059.54114 |     223.60823
-------+------------+---------------+---------------+---------------+---------------
 Total |       6574 |   24679.03037 |       5.85630 |    2373.00942 |     583.22930
`
	values, err := extractCompactionValues(stats)

	require.Equal(t, nil, err)
	require.Equal(t, "0", values[0].Level)
	require.Equal(t, int64(27), values[1].Tables)
	require.Equal(t, int64(989), values[2].SizeMB)
}

func TestParseAndRoundFloatToInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		hasError bool
	}{
		{"integer", "42", 42, false},
		{"decimal", "42.51", 43, false},
		{"negative decimal", "-42.51", -43, false},
		{"zero", "0.0", 0, false},
		{"very small decimal", "0.1", 0, false},
		{"very large number", "9999999.999", 10000000, false},
		{"invalid format", "not_a_number", 0, true},
		{"empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAndRoundFloatToInt64(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestExtractCompactionValues(t *testing.T) {
	tests := []struct {
		name     string
		stats    string
		expected []CompactionValues
		hasError bool
	}{
		{
			name: "valid stats",
			stats: ` Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
-------+------------+---------------+---------------+---------------+---------------
   0   |          2 |     224.46577 |       3.25844 |       0.00000 |    1908.26756
   1   |         29 |     110.98547 |       6.76062 |    2070.73768 |    2054.52797
-------+------------+---------------+---------------+---------------+---------------
 Total |         31 |     335.45124 |      10.01906 |    2070.73768 |    3962.79553
`,
			expected: []CompactionValues{
				{Level: "0", Tables: 2, SizeMB: 224, TimeSec: 3, ReadMB: 0, WriteMB: 1908},
				{Level: "1", Tables: 29, SizeMB: 111, TimeSec: 7, ReadMB: 2071, WriteMB: 2055},
			},
			hasError: false,
		},
		{
			name:     "empty stats",
			stats:    "",
			expected: nil,
			hasError: true,
		},
		{
			name: "malformed stats - missing columns",
			stats: ` Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
-------+------------+---------------+---------------+---------------+---------------
   0   |          2 |     224.46577
-------+------------+---------------+---------------+---------------+---------------
`,
			expected: nil,
			hasError: true,
		},
		{
			name: "malformed stats - invalid numbers",
			stats: ` Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
-------+------------+---------------+---------------+---------------+---------------
   0   |      abc   |     224.46577 |       3.25844 |       0.00000 |    1908.26756
-------+------------+---------------+---------------+---------------+---------------
`,
			expected: nil,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := extractCompactionValues(tt.stats)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.expected), len(values))
				for i, expected := range tt.expected {
					assert.Equal(t, expected.Level, values[i].Level)
					assert.Equal(t, expected.Tables, values[i].Tables)
					assert.Equal(t, expected.SizeMB, values[i].SizeMB)
					assert.Equal(t, expected.TimeSec, values[i].TimeSec)
					assert.Equal(t, expected.ReadMB, values[i].ReadMB)
					assert.Equal(t, expected.WriteMB, values[i].WriteMB)
				}
			}
		})
	}
}

func TestParseCompactionColumns(t *testing.T) {
	tests := []struct {
		name     string
		columns  []string
		expected *CompactionValues
		hasError bool
	}{
		{
			name:    "valid columns",
			columns: []string{"0", "|", "2", "|", "224.46577", "|", "3.25844", "|", "0.00000", "|", "1908.26756", "|"},
			expected: &CompactionValues{
				Level:   "0",
				Tables:  2,
				SizeMB:  224,
				TimeSec: 3,
				ReadMB:  0,
				WriteMB: 1908,
			},
			hasError: false,
		},
		{
			name:     "invalid 1 column",
			columns:  []string{"0", "|", "invalid", "|", "224.46577", "|", "3.25844", "|", "0.00000", "|", "1908.26756"},
			expected: nil,
			hasError: true,
		},
		{
			name:     "invalid 2 column",
			columns:  []string{"0", "|", "2", "|", "invalid", "|", "3.25844", "|", "0.00000", "|", "1908.26756"},
			expected: nil,
			hasError: true,
		},
		{
			name:     "invalid 3 column",
			columns:  []string{"0", "|", "2", "|", "224.46577", "|", "invalid", "|", "0.00000", "|", "1908.26756"},
			expected: nil,
			hasError: true,
		},
		{
			name:     "invalid 4 column",
			columns:  []string{"0", "|", "2", "|", "224.46577", "|", "3.25844", "|", "invalid", "|", "1908.26756"},
			expected: nil,
			hasError: true,
		},
		{
			name:     "invalid 5 column",
			columns:  []string{"0", "|", "2", "|", "224.46577", "|", "3.25844", "|", "0.00000", "|", "invalid"},
			expected: nil,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCompactionColumns(tt.columns)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestCollectCompactionValues(t *testing.T) {
	validStats := ` Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
-------+------------+---------------+---------------+---------------+---------------
   0   |          2 |     224.46577 |       3.25844 |       0.00000 |    1908.26756
-------+------------+---------------+---------------+---------------+---------------
 Total |          2 |     224.46577 |       3.25844 |       0.00000 |    1908.26756
`
	require.NotPanics(t, func() { collectCompactionValues(validStats) })

	invalidStats := "invalid stats format"
	require.NotPanics(t, func() { collectCompactionValues(invalidStats) })

	require.NotPanics(t, func() { collectCompactionValues("") })
}
