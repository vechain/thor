package telemetry

import (
	"net/http"
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "node_telemetry"

// InitializePrometheusTelemetry creates a new instance of the Prometheus service and
// sets the implementation as the default telemetry services
func InitializePrometheusTelemetry() {
	// don't allow for reset
	if _, ok := telemetry.(*prometheusTelemetry); !ok {
		telemetry = newPrometheusTelemetry()
	}
}

type prometheusTelemetry struct {
	counters   sync.Map
	histograms sync.Map
}

func newPrometheusTelemetry() Telemetry {
	return &prometheusTelemetry{
		counters:   sync.Map{},
		histograms: sync.Map{},
	}
}

func (o *prometheusTelemetry) GetOrCreateCountMeter(name string) CountMeter {
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

func (o *prometheusTelemetry) GetOrCreateHandler() http.Handler {
	return promhttp.Handler()
}

func (o *prometheusTelemetry) GetOrCreateHistogramMeter(name string, buckets []int64) HistogramMeter {
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

func (o *prometheusTelemetry) newHistogramMeter(name string, buckets []int64) HistogramMeter {
	var floatBuckets []float64
	for _, bucket := range buckets {
		floatBuckets = append(floatBuckets, float64(bucket))
	}

	histogram := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      name,
			Buckets:   floatBuckets,
		},
	)

	err := prometheus.Register(histogram)
	if err != nil {
		log.Warn("unable to register metric", "err", err)
	}

	return &promHistogramMeter{
		histogram: histogram,
	}
}

type promHistogramMeter struct {
	histogram prometheus.Histogram
}

func (c *promHistogramMeter) Observe(i int64) {
	c.histogram.Observe(float64(i))
}

func (o *prometheusTelemetry) newCountMeter(name string) CountMeter {
	counter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      name,
		},
	)

	err := prometheus.Register(counter)
	if err != nil {
		log.Warn("unable to register metric", "err", err)
	}
	return &promCountMeter{
		counter: counter,
	}
}

type promCountMeter struct {
	counter prometheus.Counter
}

func (c *promCountMeter) Add(i int64) {
	c.counter.Add(float64(i))
}
