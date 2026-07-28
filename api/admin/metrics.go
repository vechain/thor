// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package admin

import "github.com/vechain/thor/v2/metrics"

// toggleCount audits every toggle change made through a Gate built by NewGate.
// Labels: feature (gate name), to ("enabled" or "disabled").
var toggleCount = metrics.LazyLoadCounterVec("admin_toggle_count", []string{"feature", "to"})

func recordToggle(feature string, enabled bool) {
	to := "disabled"
	if enabled {
		to = "enabled"
	}
	toggleCount().AddWithLabel(1, map[string]string{"feature": feature, "to": to})
}
