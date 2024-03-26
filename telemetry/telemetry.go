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

// HistogramVecMeter //todo
type HistogramVecMeter interface {
	ObserveWithLabels(int64, map[string]string)
}

func HistogramVec(name string, labels []string) HistogramVecMeter {
	return telemetry.GetOrCreateHistogramVecMeter(name, labels, nil)
}
func HistogramVecWithHTTPBuckets(name string, labels []string) HistogramVecMeter {
	return telemetry.GetOrCreateHistogramVecMeter(name, labels, defaultHTTPBuckets)
}

var defaultHTTPBuckets = []int64{0, 150, 300, 450, 600, 900, 1200, 1500, 3000}

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
