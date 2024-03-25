package p2psrv

import "github.com/vechain/thor/v2/telemetry"

var (
	metricConnectedPeers  = telemetry.GaugeVec("p2p_connection_count", []string{"status"})
	metricDiscoveredNodes = telemetry.GaugeVec("p2p_discovered_node_count", []string{"status"})
	metricDialNewNode     = telemetry.Counter("p2p_dial_new_node_count")
)
