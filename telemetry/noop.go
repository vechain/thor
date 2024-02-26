package telemetry

import "net/http"

// noopTelemetry implements a no operations telemetry service
type noopTelemetry struct{}

func defaultNoopTelemetry() Telemetry { return &noopTelemetry{} }

func (n *noopTelemetry) startServer() error { return nil }

func (n *noopTelemetry) stopServer() error { return nil }

func (n *noopTelemetry) GetOrCreateHistogramMeter(string, []int64) HistogramMeter { return &noopMetric }

func (n *noopTelemetry) GetOrCreateCountMeter(string) CountMeter { return &noopMetric }

func (n *noopTelemetry) GetOrCreateHandler() http.Handler { return nil }

var noopMetric = noopMeters{}

type noopMeters struct{}

func (n noopMeters) Add(int64) {}

func (n noopMeters) Observe(int64) {}
