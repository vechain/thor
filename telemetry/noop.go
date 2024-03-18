package telemetry

import "net/http"

// noopTelemetry implements a no operations telemetry service
type noopTelemetry struct{}

func (n *noopTelemetry) GetOrCreateHistogramVecMeter(name string, labels []string, buckets []int64) HistogramVecMeter {
	return &noopTelemetry{}
}

func defaultNoopTelemetry() Telemetry { return &noopTelemetry{} }

func (n *noopTelemetry) GetOrCreateHistogramMeter(string, []int64) HistogramMeter { return &noopMetric }

func (n *noopTelemetry) GetOrCreateCountMeter(string) CountMeter { return &noopMetric }

func (n *noopTelemetry) GetOrCreateCountVecMeter(_ string, _ []string) CountVecMeter {
	return &noopMetric
}

func (n *noopTelemetry) GetOrCreateGaugeVecMeter(name string, labels []string) GaugeVecMeter {
	return &noopMetric
}

func (n *noopTelemetry) GetOrCreateHandler() http.Handler { return nil }

var noopMetric = noopMeters{}

type noopMeters struct{}

func (n noopMeters) GaugeWithLabel(i int64, m map[string]string) {}

func (n noopMeters) AddWithLabel(i int64, m map[string]string) {}

func (n noopMeters) Add(int64) {}

func (n noopMeters) Observe(int64) {}

func (n *noopTelemetry) ObserveWithLabels(i int64, m map[string]string) {}
