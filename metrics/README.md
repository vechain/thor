# Metrics Package

## Overview
The Metrics package provides a flexible and efficient way to instrument Go applications for metrics collection. It supports various metric types and is designed around a singleton pattern to ensure global access and performance. The package defaults to a no-operation implementation for environments where metrics is not required or enabled.

## Features
- **Singleton Access**: Ensures that the entire application uses a single instance of the metrics system.
- **Extensible Interface**: Supports multiple backend implementations.
- **HTTP Metrics Handler**: Easy integration with HTTP servers to expose metrics.
- **Lazy Loading**: Metrics are instantiated only when they are first used, which optimizes resource usage.

## Metric Types
- **CountMeter**: Monotonically increasing counter that resets on application restart.
- **CountVecMeter**: CountMeter with support for labeling for dimensional data.
- **GaugeMeter**: Represents a metric that can go up or down.
- **GaugeVecMeter**: GaugeMeter with label support.
- **HistogramMeter**: Measures distributions of values into predefined buckets.
- **HistogramVecMeter**: HistogramMeter with label support.

## Usage

### Counters
To create a counter:
```go
counter := metrics.Counter("request_count")
counter.Add(1)
```

For counters with labels:
```go
counterVec := metrics.CounterVec("request_count_by_status", []string{"status"})
counterVec.AddWithLabel(1, map[string]string{"status": "200"})
```

### Gauges
To create a gauge:
```go
gauge := metrics.Gauge("current_users")
gauge.Gauge(5)
```

For gauges with labels:
```go
gaugeVec := metrics.GaugeVec("current_users_by_tier", []string{"tier"})
gaugeVec.GaugeWithLabel(5, map[string]string{"tier": "premium"})
```

### Histograms
To create a histogram:
```go
histogram := metrics.Histogram("response_times_ms", metrics.Bucket10s)
histogram.Observe(350)
```

For histograms with labels:
```go
histogramVec := metrics.HistogramVec("response_times_by_route_ms", []string{"route"}, metrics.BucketHTTPReqs)
histogramVec.ObserveWithLabels(350, map[string]string{"route": "/api/data"})
```

### Lazy Loading
To defer the instantiation of any metric:
```go
lazyHistogram := metrics.LazyLoadHistogram("response_times_ms", metrics.Bucket10s)
h := lazyHistogram() // Actual instantiation occurs here
h.Observe(500)
```

## HTTP Handler
To expose metrics via HTTP:
```go
http.Handle("/metrics", metrics.Handler())
```
