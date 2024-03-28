package p2psrv

import "github.com/vechain/thor/v2/telemetry"

var (
	metricConnectedPeers  = telemetry.LazyLoadGauge("p2p_connected_peers_count")
	metricDiscoveredNodes = telemetry.LazyLoadGauge("p2p_discovered_node_count")
	metricDialNewNode     = telemetry.LazyLoadCounter("p2p_dial_new_node_count")
)
