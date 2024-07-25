// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package metrics

import "net/http"

// noopMetrics implements a no operations metrics service
type noopMetrics struct{}

func defaultNoopMetrics() Metrics { return &noopMetrics{} }

func (n *noopMetrics) GetOrCreateHistogramMeter(string, []int64) HistogramMeter { return &noopMetric }
func (n *noopMetrics) GetOrCreateHistogramVecMeter(string, []string, []int64) HistogramVecMeter {
	return &noopMetric
}
func (n *noopMetrics) GetOrCreateCountMeter(string) CountMeter { return &noopMetric }

func (n *noopMetrics) GetOrCreateCountVecMeter(string, []string) CountVecMeter {
	return &noopMetric
}

func (n *noopMetrics) GetOrCreateGaugeMeter(string) GaugeMeter {
	return &noopMetric
}
func (n *noopMetrics) GetOrCreateGaugeVecMeter(string, []string) GaugeVecMeter {
	return &noopMetric
}

func (n *noopMetrics) GetOrCreateHandler() http.Handler { return nil }

var noopMetric = noopMeters{}

type noopMeters struct{}

func (n noopMeters) ObserveWithLabels(i int64, m map[string]string) {}

func (n noopMeters) AddWithLabel(int64, map[string]string) {}

func (n noopMeters) SetWithLabel(int64, map[string]string) {}

func (n noopMeters) Add(int64) {}

func (n noopMeters) Set(int64) {}

func (n noopMeters) Observe(int64) {}

func (n *noopMetrics) ObserveWithLabels(int64, map[string]string) {}
