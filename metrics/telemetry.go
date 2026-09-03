// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metrics

import (
	"net/http"
	"sync"
)

// metrics is a singleton service that provides global access to a set of meters
// it wraps multiple implementations and defaults to a no-op implementation
var metrics = defaultNoopMetrics() // defaults to a Noop implementation of the metrics service

// Metrics defines the interface for metrics service implementations
type Metrics interface {
	GetOrCreateCountMeter(name string) CountMeter
	GetOrCreateCountVecMeter(name string, labels []string) CountVecMeter
	GetOrCreateGaugeMeter(name string) GaugeMeter
	GetOrCreateGaugeVecMeter(name string, labels []string) GaugeVecMeter
	GetOrCreateHistogramMeter(name string, buckets []int64) HistogramMeter
	GetOrCreateHistogramVecMeter(name string, labels []string, buckets []int64) HistogramVecMeter
	GetOrCreateHandler() http.Handler
}

// HTTPHandler returns the http handler for retrieving metrics
func HTTPHandler() http.Handler {
	return metrics.GetOrCreateHandler()
}

// Define standard buckets for histograms
var (
	Bucket10s      = []int64{0, 500, 1000, 2000, 3000, 4000, 5000, 7500, 10_000}
	BucketHTTPReqs = []int64{
		0, 1, 2, 5, 10, 20, 30, 50, 75, 100,
		150, 200, 300, 400, 500, 750, 1000,
		1500, 2000, 3000, 4000, 5000, 10000,
	}
)

// HistogramMeter represents the type of metric that is calculated by aggregating
// as a Histogram of all reported measurements over a time interval.
type HistogramMeter interface {
	Observe(int64)
}

func Histogram(name string, buckets []int64) HistogramMeter {
	return metrics.GetOrCreateHistogramMeter(name, buckets)
}

// HistogramVecMeter same as the Histogram but with labels
type HistogramVecMeter interface {
	ObserveWithLabels(int64, map[string]string)
}

func HistogramVec(name string, labels []string, buckets []int64) HistogramVecMeter {
	return metrics.GetOrCreateHistogramVecMeter(name, labels, buckets)
}

// CountMeter is a cumulative metric that represents a single monotonically increasing counter
// whose value can only increase or be reset to zero on restart.
type CountMeter interface {
	Add(int64)
}

func Counter(name string) CountMeter { return metrics.GetOrCreateCountMeter(name) }

// CountVecMeter is a cumulative metric that represents a single monotonically increasing counter
// whose value can only increase or be reset to zero on restart with a vector of values.
type CountVecMeter interface {
	AddWithLabel(int64, map[string]string)
}

func CounterVec(name string, labels []string) CountVecMeter {
	return metrics.GetOrCreateCountVecMeter(name, labels)
}

// GaugeMeter is a metric that represents a single numeric value, which can arbitrarily go up and down.
type GaugeMeter interface {
	Add(int64)
	Set(int64)
}

func Gauge(name string) GaugeMeter {
	return metrics.GetOrCreateGaugeMeter(name)
}

// GaugeVecMeter is a metric that represents a single numeric value, which can arbitrarily go up and down
// with multiple labels.
type GaugeVecMeter interface {
	AddWithLabel(int64, map[string]string)
	SetWithLabel(int64, map[string]string)
}

func GaugeVec(name string, labels []string) GaugeVecMeter {
	return metrics.GetOrCreateGaugeVecMeter(name, labels)
}

// LazyLoad allows to defer the instantiation of the metric while allowing its definition. More clearly:
// - it allow metrics to be defined and used package wide (using var)
// - it avoid metrics definition to determine the singleton to use (noop vs prometheus)
func LazyLoad[T any](f func() T) func() T {
	var result T
	var once sync.Once
	return func() T {
		once.Do(func() {
			result = f()
		})
		return result
	}
}

func LazyLoadHistogram(name string, buckets []int64) func() HistogramMeter {
	return LazyLoad(func() HistogramMeter {
		return Histogram(name, buckets)
	})
}

func LazyLoadHistogramVec(name string, labels []string, buckets []int64) func() HistogramVecMeter {
	return LazyLoad(func() HistogramVecMeter {
		return HistogramVec(name, labels, buckets)
	})
}

func LazyLoadCounter(name string) func() CountMeter {
	return LazyLoad(func() CountMeter {
		return Counter(name)
	})
}

func LazyLoadCounterVec(name string, labels []string) func() CountVecMeter {
	return LazyLoad(func() CountVecMeter {
		return CounterVec(name, labels)
	})
}

func LazyLoadGaugeVec(name string, labels []string) func() GaugeVecMeter {
	return LazyLoad(func() GaugeVecMeter {
		return GaugeVec(name, labels)
	})
}

func LazyLoadGauge(name string) func() GaugeMeter {
	return LazyLoad(func() GaugeMeter {
		return Gauge(name)
	})
}
