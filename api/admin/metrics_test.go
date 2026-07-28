// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import (
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/metrics"
)

// counterValue reads thor_metrics_admin_toggle_count{feature=feature,to=to}
// from the prometheus default registry. Returns 0 if the series doesn't exist.
func counterValue(t *testing.T, feature, to string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != "thor_metrics_admin_toggle_count" {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["feature"] == feature && labels["to"] == to {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestNewGateAuditsToggles(t *testing.T) {
	metrics.InitializePrometheusMetrics()

	gate := NewGate("featuregate-audit-test", &atomic.Bool{})

	enabledBefore := counterValue(t, "featuregate-audit-test", "enabled")
	disabledBefore := counterValue(t, "featuregate-audit-test", "disabled")

	gate.Set(true, 0)
	gate.Set(false, 0)
	gate.Set(true, 0)

	require.Equal(t, enabledBefore+2, counterValue(t, "featuregate-audit-test", "enabled"))
	require.Equal(t, disabledBefore+1, counterValue(t, "featuregate-audit-test", "disabled"))
}
