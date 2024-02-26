package telemetry

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"

	metricsdk "go.opentelemetry.io/otel/sdk/metric"
)

// InitializeOtelTelemetry creates a new instance of the OpenTelemetry service and
// sets the implementation as the default telemetry services
func InitializeOtelTelemetry() {
	// don't allow for reset
	if _, ok := telemetry.(*otelPrometheus); !ok {
		telemetry = newOtelPrometheusTelemetry()
	}
}

type otelPrometheus struct {
	meter      metric.Meter
	counters   sync.Map
	histograms sync.Map
}

func newOtelPrometheusTelemetry() Telemetry {
	exporter, err := prometheus.New()
	if err != nil {
		log.Fatal(err)
	}
	provider := metricsdk.NewMeterProvider(metricsdk.WithReader(exporter))

	return &otelPrometheus{
		meter:      provider.Meter("node_telemetry"),
		counters:   sync.Map{},
		histograms: sync.Map{},
	}
}

func (o *otelPrometheus) GetOrCreateCountMeter(name string) CountMeter {
	var meter CountMeter
	mapItem, ok := o.counters.Load(name)
	if !ok {
		meter = o.newCountMeter(name)
		o.counters.Store(name, meter)
	} else {
		meter = mapItem.(CountMeter)
	}
	return meter
}

func (o *otelPrometheus) GetOrCreateHandler() http.Handler {
	return promhttp.Handler()
}

func (o *otelPrometheus) GetOrCreateHistogramMeter(name string, buckets []int64) HistogramMeter {
	var meter HistogramMeter
	mapItem, ok := o.histograms.Load(name)
	if !ok {
		meter = o.newHistogramMeter(name, buckets)
		o.histograms.Store(name, meter)
	} else {
		meter = mapItem.(HistogramMeter)
	}
	return meter
}

func (o *otelPrometheus) newHistogramMeter(name string, buckets []int64) HistogramMeter {
	var floatBuckets []float64
	for _, bucket := range buckets {
		floatBuckets = append(floatBuckets, float64(bucket))
	}
	// purposefully ignoring the error given its strict properties
	hist, _ := o.meter.Int64Histogram(name, metric.WithExplicitBucketBoundaries(floatBuckets...))

	return &otelHistogramMeter{
		histogram: hist,
	}
}

type otelHistogramMeter struct {
	histogram metric.Int64Histogram
}

func (c *otelHistogramMeter) Observe(i int64) {
	c.histogram.Record(context.Background(), i)
}

func (o *otelPrometheus) newCountMeter(name string) CountMeter {
	// purposefully ignoring the error given only the name is supplied
	counter, _ := o.meter.Int64Counter(name)

	return &otelCountMeter{
		counter: counter,
	}
}

type otelCountMeter struct {
	counter metric.Int64Counter
}

func (c *otelCountMeter) Add(i int64) {
	c.counter.Add(context.Background(), i)
}
