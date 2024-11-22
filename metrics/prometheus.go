// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metrics

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vechain/thor/v2/log"
)

var logger = log.WithContext("pkg", "metrics")

const namespace = "thor_metrics"

// InitializePrometheusMetrics creates a new instance of the Prometheus service and
// sets the implementation as the default metrics services
func InitializePrometheusMetrics() {
	// don't allow for reset
	if _, ok := metrics.(*prometheusMetrics); !ok {
		metrics = newPrometheusMetrics()
		// collection disk io metrics every 5 seconds
		go metrics.(*prometheusMetrics).collectDiskIO(5 * time.Second)
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
		logger.Warn("unable to register metric", "err", err)
	}

	return &promHistogramMeter{
		histogram: meter,
	}
}

func getIOLineValue(line string) (int64) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		logger.Warn("this io file line is malformed", "err", line)
		return 0
	}
	value, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		logger.Warn("unable to parse int", "err", err)
		return 0
	}

	return value
}

func getDiskIOData() (int64, int64, error) {
	pid := os.Getpid()
	ioFilePath := fmt.Sprintf("/proc/%d/io", pid)
	file, err := os.Open(ioFilePath)
	if err != nil {
		return 0, 0, err
	}

	// Parse the file line by line
	scanner := bufio.NewScanner(file)
	var reads, writes int64
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "syscr") {
			reads = getIOLineValue(line)
		} else if strings.HasPrefix(line, "syscw") {
			writes = getIOLineValue(line)
		}
	}

	return reads, writes, nil
}

func (o *prometheusMetrics) collectDiskIO(refresh time.Duration) {
	for {
		reads, writes, err := getDiskIOData()
		if err != nil {
			continue
		} else {
			readsMeter := o.GetOrCreateGaugeMeter("disk_reads")
			readsMeter.Set(reads)

			writesMeter := o.GetOrCreateGaugeMeter("disk_writes")
			writesMeter.Set(writes)
		}

		time.Sleep(refresh)
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
		logger.Warn("unable to register metric", "err", err)
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
		logger.Warn("unable to register metric", "err", err)
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
		logger.Warn("unable to register metric", "err", err)
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
		logger.Warn("unable to register metric", "err", err)
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
		logger.Warn("unable to register metric", "err", err)
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

func (c *promGaugeVecMeter) AddWithLabel(i int64, labels map[string]string) {
	c.gauge.With(labels).Add(float64(i))
}

func (c *promGaugeVecMeter) SetWithLabel(i int64, labels map[string]string) {
	c.gauge.With(labels).Set(float64(i))
}
