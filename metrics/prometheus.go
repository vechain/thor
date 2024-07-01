// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metrics

import (
	"net/http"
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "thor_metrics"

// InitializePrometheusMetrics creates a new instance of the Prometheus service and
// sets the implementation as the default metrics services
func InitializePrometheusMetrics() {
	// don't allow for reset
	if _, ok := metrics.(*prometheusMetrics); !ok {
		metrics = newPrometheusMetrics()
	}
}

type prometheusMetrics struct {
	counters      sync.Map
	counterVecs   sync.Map
	histograms    sync.Map
	histogramVecs sync.Map
	gaugeVecs     sync.Map
	gauges        sync.Map
}

func newPrometheusMetrics() Metrics {
	return &prometheusMetrics{
		counters:      sync.Map{},
		counterVecs:   sync.Map{},
		histograms:    sync.Map{},
		histogramVecs: sync.Map{},
		gauges:        sync.Map{},
		gaugeVecs:     sync.Map{},
	}
}

func (o *prometheusMetrics) GetOrCreateCountMeter(name string) CountMeter {
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

func (o *prometheusMetrics) GetOrCreateCountVecMeter(name string, labels []string) CountVecMeter {
	var meter CountVecMeter
	mapItem, ok := o.counterVecs.Load(name)
	if !ok {
		meter = o.newCountVecMeter(name, labels)
		o.counterVecs.Store(name, meter)
	} else {
		meter = mapItem.(CountVecMeter)
	}
	return meter
}

func (o *prometheusMetrics) GetOrCreateHandler() http.Handler {
	return promhttp.Handler()
}

func (o *prometheusMetrics) GetOrCreateHistogramMeter(name string, buckets []int64) HistogramMeter {
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

func (o *prometheusMetrics) GetOrCreateHistogramVecMeter(name string, labels []string, buckets []int64) HistogramVecMeter {
	var meter HistogramVecMeter
	mapItem, ok := o.histogramVecs.Load(name)
	if !ok {
		meter = o.newHistogramVecMeter(name, labels, buckets)
		o.histogramVecs.Store(name, meter)
	} else {
		meter = mapItem.(HistogramVecMeter)
	}
	return meter
}

func (o *prometheusMetrics) GetOrCreateGaugeMeter(name string) GaugeMeter {
	var meter GaugeMeter
	mapItem, ok := o.gauges.Load(name)
	if !ok {
		meter = o.newGaugeMeter(name)
		o.gauges.Store(name, meter)
	} else {
		meter = mapItem.(GaugeMeter)
	}
	return meter
}

func (o *prometheusMetrics) GetOrCreateGaugeVecMeter(name string, labels []string) GaugeVecMeter {
	var meter GaugeVecMeter
	mapItem, ok := o.gaugeVecs.Load(name)
	if !ok {
		meter = o.newGaugeVecMeter(name, labels)
		o.gaugeVecs.Store(name, meter)
	} else {
		meter = mapItem.(GaugeVecMeter)
	}
	return meter
}

func (o *prometheusMetrics) newHistogramMeter(name string, buckets []int64) HistogramMeter {
	var floatBuckets []float64
	for _, bucket := range buckets {
		floatBuckets = append(floatBuckets, float64(bucket))
	}

	meter := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      name,
			Buckets:   floatBuckets,
		},
	)

	err := prometheus.Register(meter)
	if err != nil {
		log.Warn("unable to register metric", "err", err)
	}

	return &promHistogramMeter{
		histogram: meter,
	}
}

type promHistogramMeter struct {
	histogram prometheus.Histogram
}

func (c *promHistogramMeter) Observe(i int64) {
	c.histogram.Observe(float64(i))
}

func (o *prometheusMetrics) newHistogramVecMeter(name string, labels []string, buckets []int64) HistogramVecMeter {
	var floatBuckets []float64
	for _, bucket := range buckets {
		floatBuckets = append(floatBuckets, float64(bucket))
	}

	meter := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      name,
			Buckets:   floatBuckets,
		},
		labels,
	)

	err := prometheus.Register(meter)
	if err != nil {
		log.Warn("unable to register metric", "err", err)
	}

	return &promHistogramVecMeter{
		histogram: meter,
	}
}

type promHistogramVecMeter struct {
	histogram *prometheus.HistogramVec
}

func (c *promHistogramVecMeter) ObserveWithLabels(i int64, labels map[string]string) {
	c.histogram.With(labels).Observe(float64(i))
}

func (o *prometheusMetrics) newCountMeter(name string) CountMeter {
	meter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      name,
		},
	)

	err := prometheus.Register(meter)
	if err != nil {
		log.Warn("unable to register metric", "err", err)
	}
	return &promCountMeter{
		counter: meter,
	}
}

func (o *prometheusMetrics) newCountVecMeter(name string, labels []string) CountVecMeter {
	meter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      name,
		},
		labels,
	)

	err := prometheus.Register(meter)
	if err != nil {
		log.Warn("unable to register metric", "err", err)
	}
	return &promCountVecMeter{
		counter: meter,
	}
}

func (o *prometheusMetrics) newGaugeMeter(name string) GaugeMeter {
	meter := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
		},
	)

	err := prometheus.Register(meter)
	if err != nil {
		log.Warn("unable to register metric", "err", err)
	}
	return &promGaugeMeter{
		gauge: meter,
	}
}

func (o *prometheusMetrics) newGaugeVecMeter(name string, labels []string) GaugeVecMeter {
	meter := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
		},
		labels,
	)

	err := prometheus.Register(meter)
	if err != nil {
		log.Warn("unable to register metric", "err", err)
	}
	return &promGaugeVecMeter{
		gauge: meter,
	}
}

type promCountMeter struct {
	counter prometheus.Counter
}

func (c *promCountMeter) Add(i int64) {
	c.counter.Add(float64(i))
}

type promCountVecMeter struct {
	counter *prometheus.CounterVec
}

func (c *promCountVecMeter) AddWithLabel(i int64, labels map[string]string) {
	c.counter.With(labels).Add(float64(i))
}

type promGaugeMeter struct {
	gauge prometheus.Gauge
}

func (c *promGaugeMeter) Add(i int64) {
	c.gauge.Add(float64(i))
}

func (c *promGaugeMeter) Set(i int64) {
	c.gauge.Set(float64(i))
}

type promGaugeVecMeter struct {
	gauge *prometheus.GaugeVec
}

func (c *promGaugeVecMeter) GaugeWithLabel(i int64, labels map[string]string) {
	c.gauge.With(labels).Add(float64(i))
}
