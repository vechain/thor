// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metrics

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// SampleKind identifies how a Sample's value should be exported.
type SampleKind int

const (
	// KindGauge is a point-in-time value that can arbitrarily go up and down.
	KindGauge SampleKind = iota
	// KindCounter is a cumulative value that only increases (until process restart).
	KindCounter
)

// Sample is a single metric data point produced by a pull-collector provider.
type Sample struct {
	Name   string            // metric name, scoped under namespace + subsystem
	Help   string            // metric help text
	Kind   SampleKind        // gauge or counter
	Labels map[string]string // optional labels
	Value  float64           // measured value
}

// RegisterPullCollector installs a Prometheus collector that samples provider on
// scrape rather than on a schedule, at most once per minInterval (0 = every
// scrape). This bounds an expensive data source (e.g. a lock-guarded DB Stats())
// regardless of scrape frequency, with no background goroutine.
//
// provider is invoked once at registration to discover the metric set, so it must
// return the same (name, label-keys) on every call; only values change.
//
// The returned unregister releases the collector (call it on Close). Registration
// is skipped, and unregister is a no-op, when Prometheus is not initialized.
func RegisterPullCollector(subsystem string, minInterval time.Duration, provider func() []Sample) (unregister func()) {
	if _, ok := metrics.(*prometheusMetrics); !ok {
		return func() {}
	}

	c := &pullCollector{
		subsystem:   subsystem,
		minInterval: minInterval,
		provider:    provider,
		descs:       make(map[string]*prometheus.Desc),
	}
	// Warm up to discover descriptors (making this a checked, and thus
	// unregisterable, collector) and seed the cache.
	c.refresh()

	if err := prometheus.Register(c); err != nil {
		logger.Warn("unable to register pull collector", "subsystem", subsystem, "err", err)
		return func() {}
	}
	return func() { prometheus.Unregister(c) }
}

// pullCollector implements prometheus.Collector.
type pullCollector struct {
	subsystem   string
	minInterval time.Duration
	provider    func() []Sample

	mu            sync.Mutex
	descs         map[string]*prometheus.Desc // key: name + label keys, discovered at registration
	cached        []Sample
	lastSampledAt time.Time
}

// Describe implements prometheus.Collector.
func (c *pullCollector) Describe(ch chan<- *prometheus.Desc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range c.descs {
		ch <- d
	}
}

// Collect implements prometheus.Collector, emitting const metrics from the
// (cached) samples with their declared value type.
func (c *pullCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if now.Sub(c.lastSampledAt) >= c.minInterval {
		c.refreshLocked()
	}

	for i := range c.cached {
		s := &c.cached[i]
		names, values := sortedLabels(s.Labels)
		desc, ok := c.descs[descKey(s.Name, names)]
		if !ok {
			// Emitting an undescribed metric would fail Gather on a checked collector.
			logger.Warn("pull collector produced an undescribed metric", "name", s.Name)
			continue
		}
		valueType := prometheus.GaugeValue
		if s.Kind == KindCounter {
			valueType = prometheus.CounterValue
		}
		metric, err := prometheus.NewConstMetric(desc, valueType, s.Value, values...)
		if err != nil {
			logger.Warn("unable to create pull metric", "name", s.Name, "err", err)
			continue
		}
		ch <- metric
	}
}

func (c *pullCollector) refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshLocked()
}

// refreshLocked samples the provider, refreshes the cache and records any new
// descriptors. The caller must hold c.mu.
func (c *pullCollector) refreshLocked() {
	samples := c.provider()
	c.cached = samples
	c.lastSampledAt = time.Now()
	for i := range samples {
		s := &samples[i]
		names, _ := sortedLabels(s.Labels)
		key := descKey(s.Name, names)
		if _, ok := c.descs[key]; !ok {
			c.descs[key] = prometheus.NewDesc(
				prometheus.BuildFQName(namespace, c.subsystem, s.Name),
				s.Help,
				names,
				nil,
			)
		}
	}
}

// descKey identifies a descriptor by metric name and its sorted label keys.
func descKey(name string, sortedLabelNames []string) string {
	return name + "\x00" + strings.Join(sortedLabelNames, "\x00")
}

// sortedLabels returns label names (sorted) and their matching values.
func sortedLabels(labels map[string]string) (names, values []string) {
	if len(labels) == 0 {
		return nil, nil
	}
	names = make([]string, 0, len(labels))
	for k := range labels {
		names = append(names, k)
	}
	sort.Strings(names)
	values = make([]string, len(names))
	for i, n := range names {
		values[i] = labels[n]
	}
	return names, values
}
