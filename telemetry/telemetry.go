package telemetry

import "net/http"

// telemetry is a singleton service that provides global access to a set of meters
// it wraps multiple implementations and defaults to a no-op implementation
var telemetry = defaultNoopTelemetry() // defaults to a Noop implementation of the telemetry service

// Telemetry defines the interface for telemetry service implementations
type Telemetry interface {
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
	return telemetry.GetOrCreateHandler()
}

// Define standard buckets for histograms
var (
	Bucket10s      = []int64{0, 500, 1000, 2000, 3000, 4000, 5000, 7500, 10_000}
	BucketHTTPReqs = []int64{0, 150, 300, 450, 600, 900, 1200, 1500, 3000}
)

// HistogramMeter represents the type of metric that is calculated by aggregating
// as a Histogram of all reported measurements over a time interval.
type HistogramMeter interface {
	Observe(int64)
}

func Histogram(name string, buckets []int64) HistogramMeter {
	return telemetry.GetOrCreateHistogramMeter(name, buckets)
}

// HistogramVecMeter same as the Histogram but with labels
type HistogramVecMeter interface {
	ObserveWithLabels(int64, map[string]string)
}

func HistogramVec(name string, labels []string, buckets []int64) HistogramVecMeter {
	return telemetry.GetOrCreateHistogramVecMeter(name, labels, buckets)
}

// CountMeter is a cumulative metric that represents a single monotonically increasing counter
// whose value can only increase or be reset to zero on restart.
type CountMeter interface {
	Add(int64)
}

func Counter(name string) CountMeter { return telemetry.GetOrCreateCountMeter(name) }

// CountVecMeter is a cumulative metric that represents a single monotonically increasing counter
// whose value can only increase or be reset to zero on restart with a vector of values.
type CountVecMeter interface {
	AddWithLabel(int64, map[string]string)
}

func CounterVec(name string, labels []string) CountVecMeter {
	return telemetry.GetOrCreateCountVecMeter(name, labels)
}

// GaugeMeter ...
type GaugeMeter interface {
	Gauge(int64)
}

func Gauge(name string) GaugeMeter {
	return telemetry.GetOrCreateGaugeMeter(name)
}

// GaugeVecMeter ...
type GaugeVecMeter interface {
	GaugeWithLabel(int64, map[string]string)
}

func GaugeVec(name string, labels []string) GaugeVecMeter {
	return telemetry.GetOrCreateGaugeVecMeter(name, labels)
}

// LazyLoad allows to defer the instantiation of the metric while allowing its definition. More clearly:
// - it allow metrics to be defined and used package wide (using var)
// - it avoid metrics definition to determine the singleton to use (noop vs prometheus)
func LazyLoad[T any](f func() T) func() T {
	var result T
	var loaded bool
	return func() T {
		if !loaded {
			result = f()
			loaded = true
		}
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
