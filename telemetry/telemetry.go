package telemetry

import "net/http"

// telemetry is a singleton service that provides global access to a set of meters
// it wraps multiple implementations and defaults to a no-op implementation
var telemetry = defaultNoopTelemetry() // defaults to a Noop implementation of the telemetry service

// Telemetry defines the interface for telemetry service implementations
type Telemetry interface {
	GetOrCreateCountMeter(name string) CountMeter
	GetOrCreateHistogramMeter(name string, buckets []int64) HistogramMeter
	GetOrCreateHandler() http.Handler
}

// Handler returns the http handler for retrieving metrics
func Handler() http.Handler {
	return telemetry.GetOrCreateHandler()
}

// HistogramMeter represents the type of metric that is calculated by aggregating
// as a Histogram of all reported measurements over a time interval.
type HistogramMeter interface {
	Observe(int64)
}

func Histogram(name string) HistogramMeter {
	return telemetry.GetOrCreateHistogramMeter(name, nil)
}
func HistogramWithHTTPBuckets(name string) HistogramMeter {
	return telemetry.GetOrCreateHistogramMeter(name, defaultHTTPBuckets)
}

var defaultHTTPBuckets = []int64{0, 150, 300, 450, 600, 900, 1200, 1500, 3000}

// CountMeter is a cumulative metric that represents a single monotonically increasing counter
// whose value can only increase or be reset to zero on restart.
type CountMeter interface {
	Add(int64)
}

func Counter(name string) CountMeter { return telemetry.GetOrCreateCountMeter(name) }
