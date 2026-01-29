// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

//go:build linux

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// test in docker
// docker run --rm -v $(pwd):/app -w /app golang:1.25 go test ./metrics/... -v -run "Process"

func TestProcessCollector_GetIOData(t *testing.T) {
	collector := NewIOCollector()

	io, err := collector.getIOStats()
	require.NoError(t, err)
	require.NotNil(t, io)

	// All IO values should be non-negative
	assert.GreaterOrEqual(t, io.readSyscalls, int64(0))
	assert.GreaterOrEqual(t, io.writeSyscalls, int64(0))
	assert.GreaterOrEqual(t, io.readBytes, int64(0))
	assert.GreaterOrEqual(t, io.writeBytes, int64(0))
}

func TestProcessCollector_Describe(t *testing.T) {
	collector := NewIOCollector()

	ch := make(chan *prometheus.Desc, 10)
	go func() {
		collector.Describe(ch)
		close(ch)
	}()

	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}

	// Should have 4 metric descriptors (I/O only)
	assert.Len(t, descs, 4)
}

func TestProcessCollector_Collect(t *testing.T) {
	collector := NewIOCollector()

	ch := make(chan prometheus.Metric, 10)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}

	// Should have 4 metrics (I/O only)
	assert.Len(t, metrics, 4)

	// Verify metric names and types
	expectedMetrics := map[string]dto.MetricType{
		"thor_metrics_process_read_syscalls_total":  dto.MetricType_COUNTER,
		"thor_metrics_process_write_syscalls_total": dto.MetricType_COUNTER,
		"thor_metrics_process_read_bytes_total":     dto.MetricType_COUNTER,
		"thor_metrics_process_write_bytes_total":    dto.MetricType_COUNTER,
	}

	for _, metric := range metrics {
		desc := metric.Desc()
		var dtoMetric dto.Metric
		err := metric.Write(&dtoMetric)
		require.NoError(t, err)

		// Get metric name from description
		descStr := desc.String()

		// Verify the metric exists in expected list
		found := false
		for name, expectedType := range expectedMetrics {
			if containsMetricName(descStr, name) {
				found = true
				// Verify type - all should be counters
				assert.Equal(t, dto.MetricType_COUNTER, expectedType)
				assert.NotNil(t, dtoMetric.Counter, "metric %s should be a counter", name)
				assert.GreaterOrEqual(t, dtoMetric.Counter.GetValue(), float64(0))
				break
			}
		}
		assert.True(t, found, "unexpected metric: %s", descStr)
	}
}

func containsMetricName(descStr, name string) bool {
	return len(descStr) > 0 && len(name) > 0 &&
		(descStr == name ||
			len(descStr) > len(name) &&
				(descStr[:len(name)] == name ||
					contains(descStr, name)))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestProcessCollector_Integration(t *testing.T) {
	// Create a new registry to avoid conflicts with default registry
	registry := prometheus.NewRegistry()

	collector := NewProcessCollector()
	err := registry.Register(collector)
	require.NoError(t, err)

	// Gather metrics
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Should have 4 metric families (I/O only)
	assert.Len(t, metricFamilies, 4)

	// Verify each metric family
	expectedTypes := map[string]dto.MetricType{
		"thor_metrics_process_read_syscalls_total":  dto.MetricType_COUNTER,
		"thor_metrics_process_write_syscalls_total": dto.MetricType_COUNTER,
		"thor_metrics_process_read_bytes_total":     dto.MetricType_COUNTER,
		"thor_metrics_process_write_bytes_total":    dto.MetricType_COUNTER,
	}

	for _, mf := range metricFamilies {
		name := mf.GetName()
		expectedType, ok := expectedTypes[name]
		require.True(t, ok, "unexpected metric family: %s", name)
		assert.Equal(t, expectedType, mf.GetType(), "metric %s has wrong type", name)
		assert.NotEmpty(t, mf.GetMetric(), "metric %s should have values", name)
	}
}
