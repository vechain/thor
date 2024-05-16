// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import "github.com/vechain/thor/v2/telemetry"

var (
	metricConnectedPeers  = telemetry.LazyLoadGauge("p2p_connected_peers_gauge")
	metricDiscoveredNodes = telemetry.LazyLoadCounter("p2p_discovered_node_count")
	metricDialingNewNode  = telemetry.LazyLoadGauge("p2p_dialing_new_node_gauge")
)
