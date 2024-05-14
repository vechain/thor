// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package p2psrv

import "github.com/vechain/thor/v2/telemetry"

var (
	metricConnectedPeers  = telemetry.LazyLoadGauge("p2p_connected_peers_count")
	metricDiscoveredNodes = telemetry.LazyLoadGauge("p2p_discovered_node_count")
	metricDialNewNode     = telemetry.LazyLoadCounter("p2p_dial_new_node_count")
)
